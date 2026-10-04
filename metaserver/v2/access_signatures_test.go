// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"encoding/hex"
	"encoding/json"
	metaserver "github.com/atrinik/protocol/metaserver/v2"
	"os"
	"strconv"
	"testing"
)

func checkCurrentSignature(t *testing.T, fixture gamePublisherFixture, profile metaserver.Profile) {
	t.Helper()
	sequence, err := strconv.ParseUint(fixture.Sequence, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	nonceBytes, err := hex.DecodeString(fixture.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	copy(nonce[:], nonceBytes)
	components, err := metaserver.Build(metaserver.Parameters{Profile: profile, Authority: fixture.Authority, ServerID: fixture.ServerID, Sequence: sequence, Nonce: nonce, Created: fixture.Created}, []byte(fixture.Body))
	if err != nil {
		t.Fatal(err)
	}
	if components.Path != fixture.Path || components.ContentDigest != fixture.ContentDigest || components.SignatureInput != fixture.SignatureInput || components.SignatureBase != fixture.SignatureBase {
		t.Fatal("canonical signature fixture mismatch")
	}
	if err := metaserver.VerifyCertificateSignature(decodeBase64(t, fixture.CertificateDERBase64), fixture.ServerID, components.SignatureBase, decodeBase64(t, fixture.SignatureBase64)); err != nil {
		t.Fatal(err)
	}
}

func TestClassicV3SignatureAndBody(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/metaserver-classic-publisher-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture gamePublisherFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	checkCurrentSignature(t, fixture, metaserver.ClassicV3Profile)
	if _, err := metaserver.ParseClassicV3PublishJSON([]byte(fixture.Body)); err != nil {
		t.Fatal(err)
	}
}

func TestAccessRouteSignatureVectors(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/access-routes-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []gamePublisherFixture `json:"signature_vectors"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Vectors) != 4 {
		t.Fatal("missing route operations")
	}
	for _, v := range fixture.Vectors {
		checkCurrentSignature(t, v, metaserver.ClassicRoutesProfile)
	}
}
