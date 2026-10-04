// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package validation_test

import (
	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
	"github.com/atrinik/protocol/validation"
	"google.golang.org/protobuf/proto"
	"os"
	"testing"
)

func TestAccessAuthGoldenAndBounds(t *testing.T) {
	data, err := os.ReadFile("../fixtures/access-auth-v1.bin")
	if err != nil {
		t.Fatal(err)
	}
	var request gamev1.AccessAuth
	if err = proto.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	session := append([]byte(nil), request.SessionId.Value...)
	if validation.AccessAuth(&request, session) != nil {
		t.Fatal("shared fixture invalid")
	}
	session[0] ^= 1
	if validation.AccessAuth(&request, session) == nil {
		t.Fatal("wrong session accepted")
	}
	session[0] ^= 1
	for _, code := range [][]byte{nil, []byte("000000000000000"), []byte("00000000000000000"), []byte("OOOOOOOOOOOOOOOO")} {
		request.Code = code
		if validation.AccessAuth(&request, session) == nil {
			t.Fatal("bad code accepted")
		}
	}
	for _, status := range []gamev1.AccessStatus{0, 3, -1} {
		if validation.AccessResult(&gamev1.AccessResult{Status: status}) == nil {
			t.Fatal("unknown result accepted")
		}
	}
	hello := &gamev1.ServerHello{Version: &gamev1.ProtocolVersion{Major: 1, Minor: 1}, Capabilities: []gamev1.Capability{gamev1.Capability_CAPABILITY_ACCESS_TOKENS_V1}, AccessPolicy: gamev1.AccessPolicy_ACCESS_POLICY_OPEN}
	if validation.AccessServerHello(hello) != nil {
		t.Fatal("open hello invalid")
	}
	hello.AccessPolicy = 0
	if validation.AccessServerHello(hello) == nil {
		t.Fatal("missing policy treated as open")
	}
	hello.AccessPolicy = gamev1.AccessPolicy_ACCESS_POLICY_PROTECTED
	hello.Capabilities = nil
	if validation.AccessServerHello(hello) == nil {
		t.Fatal("missing capability accepted")
	}
}
