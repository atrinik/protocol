// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	metaserverv1 "github.com/atrinik/protocol/gen/go/atrinik/metaserver/v1"
)

const (
	ClassicV2PublishSchema         = "atrinik-classic-publish-v2"
	MaximumClassicVersionBytes     = 32
	MaximumClassicTextCommentBytes = 256
	MaximumClassicPlayers          = uint64(1<<32 - 1)
)

// ClassicPublishEndpoint is the explicitly configured canonical DNS endpoint
// in a Classic v2 publication. Numeric and discovered addresses are invalid.
type ClassicPublishEndpoint struct {
	Hostname string
	Port     uint32
}

// ClassicV2PublishRequest is one complete canonical Classic v2 publisher
// body. ParseClassicV2PublishJSON owns every returned allocation; callers of
// MarshalClassicV2PublishJSON retain ownership of their input.
type ClassicV2PublishRequest struct {
	ServerID           []byte
	CertificateDER     []byte
	Name               string
	PlayersCount       uint32
	Version            string
	TextComment        string
	Public             bool
	AccessCodeRequired bool
	Endpoint           *ClassicPublishEndpoint
}

// ClassicPublishErrorCode is a stable, bounded conformance failure class. It
// never contains rejected input or certificate material.
type ClassicPublishErrorCode string

const (
	ClassicPublishInvalidJSON        ClassicPublishErrorCode = "invalid_json"
	ClassicPublishNonCanonicalJSON   ClassicPublishErrorCode = "noncanonical_json"
	ClassicPublishUnsupportedSchema  ClassicPublishErrorCode = "unsupported_schema"
	ClassicPublishBodyTooLarge       ClassicPublishErrorCode = "body_too_large"
	ClassicPublishInvalidIdentity    ClassicPublishErrorCode = "invalid_identity"
	ClassicPublishInvalidCertificate ClassicPublishErrorCode = "invalid_certificate"
	ClassicPublishInvalidText        ClassicPublishErrorCode = "invalid_text"
	ClassicPublishInvalidPlayers     ClassicPublishErrorCode = "invalid_players"
	ClassicPublishInvalidEndpoint    ClassicPublishErrorCode = "invalid_endpoint"
)

// ClassicPublishError reports only a stable class suitable for bounded
// diagnostics.
type ClassicPublishError struct {
	Code ClassicPublishErrorCode
}

func (err *ClassicPublishError) Error() string {
	return "invalid Classic v2 publication: " + string(err.Code)
}

// ClassicPublishErrorCodeOf returns a bounded error class and false for
// unrelated errors.
func ClassicPublishErrorCodeOf(err error) (ClassicPublishErrorCode, bool) {
	var publishError *ClassicPublishError
	if !errors.As(err, &publishError) {
		return "", false
	}
	return publishError.Code, true
}

type classicV2PublishWire struct {
	Schema             string  `json:"schema"`
	ServerID           string  `json:"serverId"`
	Certificate        string  `json:"certificate"`
	Name               string  `json:"name"`
	PlayersCount       uint64  `json:"playersCount"`
	Version            string  `json:"version"`
	TextComment        string  `json:"textComment"`
	Public             bool    `json:"public"`
	AccessCodeRequired bool    `json:"accessCodeRequired"`
	Hostname           *string `json:"hostname,omitempty"`
	Port               *uint32 `json:"port,omitempty"`
}

// ParseClassicV2PublishJSON transactionally validates one complete canonical
// Classic v2 body. Failure returns no partial model.
func ParseClassicV2PublishJSON(input []byte) (*ClassicV2PublishRequest, error) {
	if len(input) > MaximumBodyBytes {
		return nil, classicPublishFailure(ClassicPublishBodyTooLarge)
	}
	if len(input) == 0 || !utf8.Valid(input) {
		return nil, classicPublishFailure(ClassicPublishInvalidJSON)
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	var wire classicV2PublishWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, classicPublishFailure(ClassicPublishInvalidJSON)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, classicPublishFailure(ClassicPublishNonCanonicalJSON)
	}

	request, err := classicV2PublishFromWire(wire)
	if err != nil {
		return nil, err
	}
	canonical, err := MarshalClassicV2PublishJSON(request)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, input) {
		return nil, classicPublishFailure(ClassicPublishNonCanonicalJSON)
	}
	return request, nil
}

// MarshalClassicV2PublishJSON validates and renders one canonical Classic v2
// body with no trailing LF or insignificant bytes.
func MarshalClassicV2PublishJSON(request *ClassicV2PublishRequest) ([]byte, error) {
	if err := ValidateClassicV2Publish(request); err != nil {
		return nil, err
	}
	wire := classicV2PublishToWire(request)
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wire); err != nil {
		return nil, classicPublishFailure(ClassicPublishInvalidJSON)
	}
	body := encoded.Bytes()
	if len(body) == 0 || body[len(body)-1] != '\n' {
		return nil, classicPublishFailure(ClassicPublishInvalidJSON)
	}
	body = unescapeClassicLineSeparators(body[:len(body)-1])
	if len(body) == 0 || len(body) > MaximumBodyBytes {
		return nil, classicPublishFailure(ClassicPublishBodyTooLarge)
	}
	return append([]byte(nil), body...), nil
}

// ValidateClassicV2Publish enforces semantic bounds, certificate identity,
// the P-256 key requirement, and the all-or-none explicit endpoint.
func ValidateClassicV2Publish(request *ClassicV2PublishRequest) error {
	if request == nil {
		return classicPublishFailure(ClassicPublishInvalidJSON)
	}
	if len(request.ServerID) != sha256.Size {
		return classicPublishFailure(ClassicPublishInvalidIdentity)
	}
	if !validClassicPublishText(request.Name, 1, MaximumDirectoryNameBytes) ||
		!validClassicPublishText(request.Version, 1, MaximumClassicVersionBytes) ||
		!validClassicPublishText(request.TextComment, 0, MaximumClassicTextCommentBytes) {
		return classicPublishFailure(ClassicPublishInvalidText)
	}
	if len(request.CertificateDER) == 0 ||
		len(request.CertificateDER) > MaximumCertificateDERBytes {
		return classicPublishFailure(ClassicPublishInvalidCertificate)
	}
	fingerprint := sha256.Sum256(request.CertificateDER)
	if !bytes.Equal(fingerprint[:], request.ServerID) {
		return classicPublishFailure(ClassicPublishInvalidIdentity)
	}
	certificate, err := x509.ParseCertificate(request.CertificateDER)
	if err != nil {
		return classicPublishFailure(ClassicPublishInvalidCertificate)
	}
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return classicPublishFailure(ClassicPublishInvalidCertificate)
	}
	if request.Endpoint != nil && !validDirectoryEndpoint(&metaserverv1.DirectEndpoint{
		Hostname: request.Endpoint.Hostname,
		Port:     request.Endpoint.Port,
	}) {
		return classicPublishFailure(ClassicPublishInvalidEndpoint)
	}
	return nil
}

func classicV2PublishFromWire(wire classicV2PublishWire) (*ClassicV2PublishRequest, error) {
	if wire.Schema != ClassicV2PublishSchema {
		return nil, classicPublishFailure(ClassicPublishUnsupportedSchema)
	}
	serverID, err := hex.DecodeString(wire.ServerID)
	if err != nil || len(serverID) != sha256.Size || wire.ServerID != hex.EncodeToString(serverID) {
		return nil, classicPublishFailure(ClassicPublishInvalidIdentity)
	}
	if wire.Certificate == "" || len(wire.Certificate) > maximumCertificateBase64Bytes {
		return nil, classicPublishFailure(ClassicPublishInvalidCertificate)
	}
	certificate, err := base64.StdEncoding.Strict().DecodeString(wire.Certificate)
	if err != nil {
		return nil, classicPublishFailure(ClassicPublishInvalidCertificate)
	}
	if wire.PlayersCount > MaximumClassicPlayers {
		return nil, classicPublishFailure(ClassicPublishInvalidPlayers)
	}
	if (wire.Hostname == nil) != (wire.Port == nil) {
		return nil, classicPublishFailure(ClassicPublishInvalidEndpoint)
	}
	request := &ClassicV2PublishRequest{
		ServerID:           serverID,
		CertificateDER:     certificate,
		Name:               wire.Name,
		PlayersCount:       uint32(wire.PlayersCount),
		Version:            wire.Version,
		TextComment:        wire.TextComment,
		Public:             wire.Public,
		AccessCodeRequired: wire.AccessCodeRequired,
	}
	if wire.Hostname != nil {
		request.Endpoint = &ClassicPublishEndpoint{
			Hostname: *wire.Hostname,
			Port:     *wire.Port,
		}
	}
	if err := ValidateClassicV2Publish(request); err != nil {
		return nil, err
	}
	return request, nil
}

func classicV2PublishToWire(request *ClassicV2PublishRequest) classicV2PublishWire {
	wire := classicV2PublishWire{
		Schema:             ClassicV2PublishSchema,
		ServerID:           hex.EncodeToString(request.ServerID),
		Certificate:        base64.StdEncoding.EncodeToString(request.CertificateDER),
		Name:               request.Name,
		PlayersCount:       uint64(request.PlayersCount),
		Version:            request.Version,
		TextComment:        request.TextComment,
		Public:             request.Public,
		AccessCodeRequired: request.AccessCodeRequired,
	}
	if request.Endpoint != nil {
		hostname := request.Endpoint.Hostname
		port := request.Endpoint.Port
		wire.Hostname = &hostname
		wire.Port = &port
	}
	return wire
}

func classicPublishFailure(code ClassicPublishErrorCode) error {
	return &ClassicPublishError{Code: code}
}

func validClassicPublishText(value string, minimum int, maximum int) bool {
	if !utf8.ValidString(value) || len(value) < minimum || len(value) > maximum {
		return false
	}
	for _, current := range value {
		if current <= '\u001f' || current == '\u007f' {
			return false
		}
	}
	return true
}

func unescapeClassicLineSeparators(input []byte) []byte {
	output := make([]byte, 0, len(input))
	for index := 0; index < len(input); {
		if input[index] != '\\' {
			output = append(output, input[index])
			index++
			continue
		}
		runEnd := index
		for runEnd < len(input) && input[runEnd] == '\\' {
			runEnd++
		}
		runLength := runEnd - index
		if runLength%2 == 1 && runEnd+5 <= len(input) {
			replacement := []byte(nil)
			switch string(input[runEnd : runEnd+5]) {
			case "u2028":
				replacement = []byte("\u2028")
			case "u2029":
				replacement = []byte("\u2029")
			}
			if replacement != nil {
				output = append(output, input[index:runEnd-1]...)
				output = append(output, replacement...)
				index = runEnd + 5
				continue
			}
		}
		output = append(output, input[index:runEnd]...)
		index = runEnd
	}
	return output
}
