// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"encoding/hex"
	"encoding/json"
	metaserver "github.com/atrinik/protocol/metaserver/v2"
	"os"
	"testing"
)

func TestAccessTokenVectors(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/access-tokens-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []struct {
			Code     string `json:"code"`
			ServerID string `json:"server_id"`
			Route    string `json:"route_capability"`
			Index    string `json:"index"`
			Verifier string `json:"verifier"`
		} `json:"vectors"`
		Invalid []string `json:"invalid_codes"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Vectors) != 3 || len(fixture.Invalid) < 8 {
		t.Fatal("incomplete access vectors")
	}
	for _, v := range fixture.Vectors {
		var identity [32]byte
		decoded, err := hex.DecodeString(v.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		copy(identity[:], decoded)
		route, err := metaserver.AccessRouteCapability([]byte(v.Code))
		if err != nil {
			t.Fatal(err)
		}
		index := metaserver.AccessRouteIndex(route)
		verifier, err := metaserver.AccessVerifier(identity, []byte(v.Code))
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(route[:]) != v.Route || hex.EncodeToString(index[:]) != v.Index || hex.EncodeToString(verifier[:]) != v.Verifier {
			t.Fatal("cross-language hash mismatch")
		}
		identity[0] ^= 1
		other, _ := metaserver.AccessVerifier(identity, []byte(v.Code))
		if other == verifier {
			t.Fatal("verifier lost server binding")
		}
	}
	for _, v := range fixture.Invalid {
		if _, err := metaserver.AccessRouteCapability([]byte(v)); err == nil {
			t.Fatal("invalid code accepted")
		}
	}
}
