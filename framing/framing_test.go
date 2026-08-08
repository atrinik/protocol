// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package framing_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/atrinik/protocol/framing"
	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
	"google.golang.org/protobuf/proto"
)

func TestGoldenControlEnvelope(t *testing.T) {
	fixtures := loadFixtures(t)
	var golden framingGolden
	for _, candidate := range fixtures.Golden {
		if candidate.ID == "gp1-control-ping-1" {
			golden = candidate
			break
		}
	}
	if golden.ID == "" {
		t.Fatal("gp1-control-ping-1 is missing from the conformance fixtures")
	}
	wantPayload, err := hex.DecodeString(golden.ProtobufHex)
	if err != nil {
		t.Fatal(err)
	}
	wantFrame, err := hex.DecodeString(golden.FrameHex)
	if err != nil {
		t.Fatal(err)
	}

	message := &gamev1.ControlEnvelope{
		Sequence: 1,
		Payload: &gamev1.ControlEnvelope_Ping{
			Ping: &gamev1.Ping{Nonce: 1},
		},
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, wantPayload) {
		t.Fatalf("protobuf payload %x, want fixture %x", payload, wantPayload)
	}
	var encoded bytes.Buffer
	if err := framing.Write(&encoded, payload, framing.MaximumGameplayFrame); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), wantFrame) {
		t.Fatalf("encoded frame %x, want fixture %x", encoded.Bytes(), wantFrame)
	}

	decoded, err := framing.Read(&encoded, framing.MaximumGameplayFrame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded payload %x, want %x", decoded, payload)
	}
}

func TestRejectsInvalidLengthsAtomically(t *testing.T) {
	errorsByName := map[string]error{
		"empty_frame":         framing.ErrEmptyFrame,
		"noncanonical_length": framing.ErrNonCanonicalSize,
		"length_overflow":     framing.ErrLengthOverflow,
		"truncated":           io.ErrUnexpectedEOF,
	}
	for _, fixture := range loadFixtures(t).Negative {
		t.Run(fixture.ID, func(t *testing.T) {
			frame, err := hex.DecodeString(fixture.FrameHex)
			if err != nil {
				t.Fatal(err)
			}
			want, exists := errorsByName[fixture.Error]
			if !exists {
				t.Fatalf("fixture has unknown error name %q", fixture.Error)
			}
			payload, err := framing.Read(bytes.NewReader(frame), 1024)
			if payload != nil {
				t.Fatalf("partial payload leaked: %x", payload)
			}
			if !errors.Is(err, want) {
				t.Fatalf("error %v, want %v", err, want)
			}
		})
	}

	payload, err := framing.Read(bytes.NewReader([]byte{0x81, 0x08}), 1024)
	if payload != nil || !errors.Is(err, framing.ErrFrameTooLarge) {
		t.Fatalf("oversized frame returned payload %x and error %v", payload, err)
	}
}

func FuzzRead(f *testing.F) {
	f.Add([]byte{0x06, 0x08, 0x01, 0x2a, 0x02, 0x08, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x1f})
	f.Fuzz(func(t *testing.T, data []byte) {
		payload, _ := framing.Read(bytes.NewReader(data), 4096)
		if len(payload) > 4096 {
			t.Fatalf("decoder exceeded limit: %d", len(payload))
		}
	})
}

type framingFixtures struct {
	Golden   []framingGolden   `json:"golden"`
	Negative []framingNegative `json:"negative"`
}

type framingGolden struct {
	ID          string `json:"id"`
	ProtobufHex string `json:"protobuf_hex"`
	FrameHex    string `json:"frame_hex"`
}

type framingNegative struct {
	ID       string `json:"id"`
	FrameHex string `json:"frame_hex"`
	Error    string `json:"error"`
}

func loadFixtures(t *testing.T) framingFixtures {
	t.Helper()
	encoded, err := os.ReadFile("../fixtures/framing.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures framingFixtures
	if err := json.Unmarshal(encoded, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}
