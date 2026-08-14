// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/atrinik/protocol/metaserver"
)

type publisherFixture struct {
	Authority            string `json:"authority"`
	Body                 string `json:"body"`
	CertificateDERBase64 string `json:"certificate_der_base64"`
	ContentDigest        string `json:"content_digest"`
	ContentType          string `json:"content_type"`
	Created              int64  `json:"created"`
	Expires              int64  `json:"expires"`
	GamePath             string `json:"game_path"`
	GameSignatureBase    string `json:"game_signature_base"`
	GameSignatureInput   string `json:"game_signature_input"`
	Method               string `json:"method"`
	Nonce                string `json:"nonce"`
	Path                 string `json:"path"`
	Profile              string `json:"profile"`
	Sequence             string `json:"sequence"`
	ServerID             string `json:"server_id"`
	SignatureBase        string `json:"signature_base"`
	SignatureBase64      string `json:"signature_base64"`
	SignatureHeader      string `json:"signature_header"`
	SignatureInput       string `json:"signature_input"`
	Version              int    `json:"version"`
}

func TestPublisherGoldenFixture(t *testing.T) {
	if metaserver.ClassicProfile != metaserver.ClassicV1Profile ||
		metaserver.ClassicSignatureTag != metaserver.ClassicV1SignatureTag {
		t.Fatal("deprecated Classic v1 API aliases changed value")
	}
	fixture := loadPublisherFixture(t)
	parameters := fixtureParameters(t, fixture, metaserver.ClassicV1Profile)
	components, err := metaserver.Build(parameters, []byte(fixture.Body))
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 || fixture.Profile != "classic" || fixture.Method != "POST" ||
		fixture.ContentType != metaserver.ContentType || fixture.Expires != fixture.Created+300 {
		t.Fatal("fixture metadata does not describe the v1 classic profile")
	}
	if components.Path != fixture.Path ||
		components.ContentDigest != fixture.ContentDigest ||
		components.SignatureInput != fixture.SignatureInput ||
		components.SignatureBase != fixture.SignatureBase {
		t.Fatal("constructed classic signature components differ from the fixture")
	}

	certificate := decodeBase64(t, fixture.CertificateDERBase64)
	signature := decodeBase64(t, fixture.SignatureBase64)
	if fixture.SignatureHeader != "atrinik=:"+fixture.SignatureBase64+":" {
		t.Fatal("fixture signature header is not canonical")
	}
	if err := metaserver.VerifyCertificateSignature(
		certificate,
		fixture.ServerID,
		components.SignatureBase,
		signature,
	); err != nil {
		t.Fatalf("fixture signature did not verify: %v", err)
	}

	parameters.Profile = metaserver.GameProfile
	game, err := metaserver.Build(parameters, []byte(fixture.Body))
	if err != nil {
		t.Fatal(err)
	}
	if game.Path != fixture.GamePath ||
		game.SignatureInput != fixture.GameSignatureInput ||
		game.SignatureBase != fixture.GameSignatureBase {
		t.Fatal("constructed Game Protocol 1 components differ from the fixture")
	}
	if err := metaserver.VerifyCertificateSignature(
		certificate,
		fixture.ServerID,
		game.SignatureBase,
		signature,
	); !errors.Is(err, metaserver.ErrInvalidSignature) {
		t.Fatalf("classic signature crossed the Game Protocol 1 domain: %v", err)
	}
}

func TestPublisherFixtureRejectsMutations(t *testing.T) {
	fixture := loadPublisherFixture(t)
	certificate := decodeBase64(t, fixture.CertificateDERBase64)
	signature := decodeBase64(t, fixture.SignatureBase64)
	mutations := []struct {
		name string
		base string
	}{
		{"body digest", strings.Replace(fixture.SignatureBase, fixture.ContentDigest, strings.Repeat("0", len(fixture.ContentDigest)), 1)},
		{"authority", strings.Replace(fixture.SignatureBase, fixture.Authority, "canary.publish.meta.atrinik.org", 1)},
		{"path", strings.Replace(fixture.SignatureBase, "/v1/classic/", "/v1/", 1)},
		{"sequence", strings.Replace(fixture.SignatureBase, fixture.Sequence, "7", 1)},
		{"protocol tag", strings.Replace(fixture.SignatureBase, metaserver.ClassicV1SignatureTag, metaserver.GameSignatureTag, 1)},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if err := metaserver.VerifyCertificateSignature(
				certificate,
				fixture.ServerID,
				mutation.base,
				signature,
			); !errors.Is(err, metaserver.ErrInvalidSignature) {
				t.Fatalf("mutated signature base verified: %v", err)
			}
		})
	}

	wrongID := "0" + fixture.ServerID[1:]
	if wrongID == fixture.ServerID {
		wrongID = "1" + fixture.ServerID[1:]
	}
	if err := metaserver.VerifyCertificateSignature(
		certificate,
		wrongID,
		fixture.SignatureBase,
		signature,
	); !errors.Is(err, metaserver.ErrInvalidIdentity) {
		t.Fatalf("wrong identity was accepted: %v", err)
	}
	corruptCertificate := append([]byte(nil), certificate...)
	corruptCertificate[len(corruptCertificate)-1] ^= 1
	if err := metaserver.VerifyCertificateSignature(
		corruptCertificate,
		fixture.ServerID,
		fixture.SignatureBase,
		signature,
	); !errors.Is(err, metaserver.ErrInvalidIdentity) {
		t.Fatalf("corrupt certificate was accepted: %v", err)
	}
	corruptSignature := append([]byte(nil), signature...)
	corruptSignature[0] ^= 1
	if err := metaserver.VerifyCertificateSignature(
		certificate,
		fixture.ServerID,
		fixture.SignatureBase,
		corruptSignature,
	); !errors.Is(err, metaserver.ErrInvalidSignature) {
		t.Fatalf("corrupt signature was accepted: %v", err)
	}
	if err := metaserver.VerifyCertificateSignature(
		make([]byte, metaserver.MaximumCertificateDERBytes+1),
		fixture.ServerID,
		fixture.SignatureBase,
		signature,
	); !errors.Is(err, metaserver.ErrInvalidIdentity) {
		t.Fatalf("oversized certificate returned %v", err)
	}
}

func TestPublisherComponentBounds(t *testing.T) {
	fixture := loadPublisherFixture(t)
	valid := fixtureParameters(t, fixture, metaserver.ClassicV1Profile)
	tests := []struct {
		name       string
		parameters metaserver.Parameters
		body       []byte
	}{
		{"empty body", valid, nil},
		{"oversized body", valid, make([]byte, metaserver.MaximumBodyBytes+1)},
		{"zero sequence", withSequence(valid, 0), []byte(fixture.Body)},
		{"upper authority", withAuthority(valid, "Publish.meta.atrinik.org"), []byte(fixture.Body)},
		{"default port", withAuthority(valid, "publish.meta.atrinik.org:443"), []byte(fixture.Body)},
		{"scheme", withAuthority(valid, "https://publish.meta.atrinik.org"), []byte(fixture.Body)},
		{"invalid profile", withProfile(valid, 0), []byte(fixture.Body)},
		{"negative created", withCreated(valid, -1), []byte(fixture.Body)},
	}
	zeroNonce := valid
	zeroNonce.Nonce = [16]byte{}
	tests = append(tests, struct {
		name       string
		parameters metaserver.Parameters
		body       []byte
	}{"zero nonce", zeroNonce, []byte(fixture.Body)})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := metaserver.Build(test.parameters, test.body); !errors.Is(err, metaserver.ErrInvalidComponent) {
				t.Fatalf("invalid component returned %v", err)
			}
		})
	}
	if _, err := metaserver.Build(valid, make([]byte, metaserver.MaximumBodyBytes)); err != nil {
		t.Fatalf("maximum body was rejected: %v", err)
	}
}

func TestPublisherSignerUsesP1363Encoding(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	base := "canonical publisher signature base"
	signature, err := metaserver.Sign(privateKey, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(signature) != 64 {
		t.Fatalf("signature length %d, want 64", len(signature))
	}
	digest := sha256.Sum256([]byte(base))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(&privateKey.PublicKey, digest[:], r, s) {
		t.Fatal("P1363 signature did not verify")
	}
	if _, err := metaserver.Sign(nil, base); !errors.Is(err, metaserver.ErrInvalidSignature) {
		t.Fatalf("nil signer returned %v", err)
	}
}

func FuzzPublisher(f *testing.F) {
	f.Add(
		"publish.meta.atrinik.org",
		strings.Repeat("a", 64),
		uint64(1),
		[]byte("0123456789abcdef"),
		int64(1),
		[]byte("{}"),
		[]byte{},
		[]byte{},
	)
	f.Fuzz(func(
		t *testing.T,
		authority string,
		serverID string,
		sequence uint64,
		nonceBytes []byte,
		created int64,
		body []byte,
		certificate []byte,
		signature []byte,
	) {
		var nonce [16]byte
		copy(nonce[:], nonceBytes)
		parameters := metaserver.Parameters{
			Profile:   metaserver.ClassicV1Profile,
			Authority: authority,
			ServerID:  serverID,
			Sequence:  sequence,
			Nonce:     nonce,
			Created:   created,
		}
		components, err := metaserver.Build(parameters, body)
		if err == nil {
			rebuilt, rebuildErr := metaserver.Build(parameters, body)
			if rebuildErr != nil || rebuilt != components || components.Path == "" ||
				components.ContentDigest == "" || components.SignatureInput == "" ||
				components.SignatureBase == "" {
				t.Fatal("successful publisher construction was not deterministic and complete")
			}
		}
		_ = metaserver.VerifyCertificateSignature(
			certificate,
			serverID,
			components.SignatureBase,
			signature,
		)
	})
}

func fixtureParameters(t *testing.T, fixture publisherFixture, profile metaserver.Profile) metaserver.Parameters {
	t.Helper()
	sequence, err := strconv.ParseUint(fixture.Sequence, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	nonceBytes, err := hex.DecodeString(fixture.Nonce)
	if err != nil || len(nonceBytes) != 16 {
		t.Fatalf("invalid fixture nonce: %v", err)
	}
	var nonce [16]byte
	copy(nonce[:], nonceBytes)
	return metaserver.Parameters{
		Profile:   profile,
		Authority: fixture.Authority,
		ServerID:  fixture.ServerID,
		Sequence:  sequence,
		Nonce:     nonce,
		Created:   fixture.Created,
	}
}

func withSequence(value metaserver.Parameters, sequence uint64) metaserver.Parameters {
	value.Sequence = sequence
	return value
}

func withAuthority(value metaserver.Parameters, authority string) metaserver.Parameters {
	value.Authority = authority
	return value
}

func withProfile(value metaserver.Parameters, profile metaserver.Profile) metaserver.Parameters {
	value.Profile = profile
	return value
}

func withCreated(value metaserver.Parameters, created int64) metaserver.Parameters {
	value.Created = created
	return value
}

func loadPublisherFixture(t *testing.T) publisherFixture {
	t.Helper()
	encoded, err := os.ReadFile("../fixtures/metaserver-publisher-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture publisherFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func decodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
