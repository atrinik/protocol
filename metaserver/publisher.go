// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

// Package metaserver implements the canonical HTTP Message Signature profile
// used by Atrinik metaserver publishers. It constructs and verifies the signed
// bytes; HTTP clients and servers remain responsible for strict field parsing,
// request limits, replay state, and publication authorization.
package metaserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	SignatureLabel             = "atrinik"
	SignatureAlgorithm         = "ecdsa-p256-sha256"
	ContentType                = "application/json"
	MaximumBodyBytes           = 4096
	MaximumCertificateDERBytes = 2048
	MaximumClockSkew           = 300
	ClassicSignatureTag        = "atrinik-classic-publish-v1"
	GameSignatureTag           = "atrinik-game-publish-v1"
	SignatureValidity          = MaximumClockSkew
	classicPathPrefix          = "/v1/classic/servers/"
	gamePathPrefix             = "/v1/servers/"
	publishPathSuffix          = "/publish"
	coveredComponents          = "(\"@method\" \"@authority\" \"@path\" \"content-digest\" \"content-type\" \"atrinik-server-id\" \"atrinik-publish-sequence\")"
	maximumSFInteger           = int64(999_999_999_999_999)
	serverIDHexLength          = 64
	p256SignatureLength        = 64
	p256CoordinateLength       = 32
)

var (
	ErrInvalidComponent = errors.New("invalid metaserver signature component")
	ErrInvalidIdentity  = errors.New("invalid metaserver certificate identity")
	ErrInvalidSignature = errors.New("invalid metaserver signature")
)

// Profile selects one independently versioned route and signature domain.
type Profile uint8

const (
	ClassicProfile Profile = iota + 1
	GameProfile
)

// Parameters are the bounded per-request values carried by Signature-Input
// and signed request fields.
type Parameters struct {
	Profile   Profile
	Authority string
	ServerID  string
	Sequence  uint64
	Nonce     [16]byte
	Created   int64
}

// Components contains every canonical value needed to send or verify one
// signed request. SignatureBase intentionally excludes a trailing newline.
type Components struct {
	Path           string
	ContentDigest  string
	SignatureInput string
	SignatureBase  string
}

// Build constructs the strict Atrinik RFC 9421 and RFC 9530 profile for the
// exact body bytes. Callers must send those bytes unchanged.
func Build(parameters Parameters, body []byte) (Components, error) {
	if len(body) == 0 || len(body) > MaximumBodyBytes ||
		!validAuthority(parameters.Authority) ||
		!validLowerHex(parameters.ServerID, serverIDHexLength) ||
		parameters.Sequence == 0 ||
		allZero(parameters.Nonce[:]) ||
		parameters.Created < 0 ||
		parameters.Created > maximumSFInteger-SignatureValidity {
		return Components{}, ErrInvalidComponent
	}

	path, tag, ok := profileValues(parameters.Profile, parameters.ServerID)
	if !ok {
		return Components{}, ErrInvalidComponent
	}
	digestBytes := sha256.Sum256(body)
	contentDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(digestBytes[:]) + ":"
	nonce := hex.EncodeToString(parameters.Nonce[:])
	expires := parameters.Created + SignatureValidity
	signatureParameters := fmt.Sprintf(
		`%s;created=%d;expires=%d;nonce="%s";alg="%s";keyid="%s";tag="%s"`,
		coveredComponents,
		parameters.Created,
		expires,
		nonce,
		SignatureAlgorithm,
		parameters.ServerID,
		tag,
	)
	signatureInput := SignatureLabel + "=" + signatureParameters
	sequence := strconv.FormatUint(parameters.Sequence, 10)
	signatureBase := strings.Join([]string{
		`"@method": POST`,
		`"@authority": ` + parameters.Authority,
		`"@path": ` + path,
		`"content-digest": ` + contentDigest,
		`"content-type": ` + ContentType,
		`"atrinik-server-id": ` + parameters.ServerID,
		`"atrinik-publish-sequence": ` + sequence,
		`"@signature-params": ` + signatureParameters,
	}, "\n")

	return Components{
		Path:           path,
		ContentDigest:  contentDigest,
		SignatureInput: signatureInput,
		SignatureBase:  signatureBase,
	}, nil
}

// Sign signs a canonical signature base and returns the RFC 9421 P-256
// signature encoding: unsigned, zero-padded r followed by s.
func Sign(privateKey *ecdsa.PrivateKey, signatureBase string) ([]byte, error) {
	if privateKey == nil || privateKey.Curve != elliptic.P256() || signatureBase == "" {
		return nil, ErrInvalidSignature
	}
	digest := sha256.Sum256([]byte(signatureBase))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		return nil, fmt.Errorf("sign metaserver request: %w", err)
	}
	return encodeP256Signature(r, s)
}

// VerifyCertificateSignature verifies that certificateDER hashes to serverID,
// carries a P-256 key, and signed the exact canonical signature base.
func VerifyCertificateSignature(
	certificateDER []byte,
	serverID string,
	signatureBase string,
	signature []byte,
) error {
	if !validLowerHex(serverID, serverIDHexLength) ||
		len(certificateDER) == 0 || len(certificateDER) > MaximumCertificateDERBytes ||
		len(signature) != p256SignatureLength {
		return ErrInvalidIdentity
	}
	fingerprint := sha256.Sum256(certificateDER)
	if hex.EncodeToString(fingerprint[:]) != serverID {
		return ErrInvalidIdentity
	}
	certificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		return ErrInvalidIdentity
	}
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return ErrInvalidIdentity
	}
	r := new(big.Int).SetBytes(signature[:p256CoordinateLength])
	s := new(big.Int).SetBytes(signature[p256CoordinateLength:])
	digest := sha256.Sum256([]byte(signatureBase))
	if r.Sign() <= 0 || s.Sign() <= 0 || !ecdsa.Verify(publicKey, digest[:], r, s) {
		return ErrInvalidSignature
	}
	return nil
}

func encodeP256Signature(r, s *big.Int) ([]byte, error) {
	if r == nil || s == nil || r.Sign() <= 0 || s.Sign() <= 0 ||
		r.BitLen() > p256CoordinateLength*8 || s.BitLen() > p256CoordinateLength*8 {
		return nil, ErrInvalidSignature
	}
	encoded := make([]byte, p256SignatureLength)
	r.FillBytes(encoded[:p256CoordinateLength])
	s.FillBytes(encoded[p256CoordinateLength:])
	return encoded, nil
}

func profileValues(profile Profile, serverID string) (string, string, bool) {
	switch profile {
	case ClassicProfile:
		return classicPathPrefix + serverID + publishPathSuffix, ClassicSignatureTag, true
	case GameProfile:
		return gamePathPrefix + serverID + publishPathSuffix, GameSignatureTag, true
	default:
		return "", "", false
	}
}

func validAuthority(value string) bool {
	if value == "" || len(value) > 253 || value != strings.ToLower(value) {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' ||
			label[len(label)-1] == '-' {
			return false
		}
		for _, current := range []byte(label) {
			if (current < 'a' || current > 'z') &&
				(current < '0' || current > '9') && current != '-' {
				return false
			}
		}
	}
	return true
}

func validLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func allZero(value []byte) bool {
	var combined byte
	for _, current := range value {
		combined |= current
	}
	return combined == 0
}
