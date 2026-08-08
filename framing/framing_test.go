// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package framing_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/atrinik/protocol/framing"
	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
	"google.golang.org/protobuf/proto"
)

func TestGoldenControlEnvelope(t *testing.T) {
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
	if want := []byte{0x08, 0x01, 0x2a, 0x02, 0x08, 0x01}; !bytes.Equal(payload, want) {
		t.Fatalf("protobuf payload %x, want %x", payload, want)
	}
	var encoded bytes.Buffer
	if err := framing.Write(&encoded, payload, framing.MaximumGameplayFrame); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x06, 0x08, 0x01, 0x2a, 0x02, 0x08, 0x01}; !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("encoded frame %x, want %x", encoded.Bytes(), want)
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
	tests := []struct {
		name  string
		frame []byte
		want  error
	}{
		{"empty", []byte{0}, framing.ErrEmptyFrame},
		{"noncanonical", []byte{0x86, 0x00}, framing.ErrNonCanonicalSize},
		{"overflow", []byte{0xff, 0xff, 0xff, 0xff, 0x1f}, framing.ErrLengthOverflow},
		{"oversized", []byte{0x81, 0x08}, framing.ErrFrameTooLarge},
		{"truncated", []byte{0x02, 0x01}, io.ErrUnexpectedEOF},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := framing.Read(bytes.NewReader(test.frame), 1024)
			if payload != nil {
				t.Fatalf("partial payload leaked: %x", payload)
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("error %v, want %v", err, test.want)
			}
		})
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
