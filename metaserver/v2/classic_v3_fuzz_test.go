// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/atrinik/protocol/metaserver/v2"
)

type classicV3FuzzFixture struct {
	Body string `json:"body"`
	Open struct {
		Body string `json:"body"`
	} `json:"open"`
	Private struct {
		Body string `json:"body"`
	} `json:"private"`
}

func FuzzClassicV3PublishJSON(f *testing.F) {
	fixture := loadClassicV3FuzzFixture(f)
	root := []byte(fixture.Body)
	f.Add(root)
	f.Add([]byte(fixture.Open.Body))
	f.Add([]byte(fixture.Private.Body))

	f.Add([]byte(`{"schema":`))
	f.Add(bytes.Clone(root[:len(root)-1]))
	f.Add(append(bytes.Clone(root[:len(root)-1]), []byte(`,"unknown":true}`)...))
	f.Add(bytes.Replace(root, []byte(`"playersCount":3`), []byte(`"playersCount":"3"`), 1))
	f.Add(append([]byte(`{"schema":"duplicate",`), root[1:]...))
	f.Add([]byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'})
	f.Add(bytes.Repeat([]byte{'x'}, metaserver.MaximumBodyBytes+1))

	f.Fuzz(func(t *testing.T, input []byte) {
		request, err := metaserver.ParseClassicV3PublishJSON(input)
		if err != nil {
			if request != nil {
				t.Fatal("failed parse returned a partial request")
			}
			code, ok := metaserver.ClassicPublishErrorCodeOf(err)
			if !ok || !boundedClassicV3PublishError(code) {
				t.Fatalf("unbounded error class: %T", err)
			}
			return
		}

		encoded, err := metaserver.MarshalClassicV3PublishJSON(request)
		if err != nil {
			t.Fatalf("accepted body failed to render: %v", err)
		}
		if !bytes.Equal(encoded, input) {
			t.Fatal("accepted body was not canonical")
		}
		roundTripped, err := metaserver.ParseClassicV3PublishJSON(encoded)
		if err != nil {
			t.Fatalf("canonical body failed to parse again: %v", err)
		}
		reencoded, err := metaserver.MarshalClassicV3PublishJSON(roundTripped)
		if err != nil || !bytes.Equal(reencoded, input) {
			t.Fatal("accepted body did not round-trip byte-identically")
		}
	})
}

func loadClassicV3FuzzFixture(t testing.TB) classicV3FuzzFixture {
	t.Helper()
	data, err := os.ReadFile("../../fixtures/metaserver-classic-publisher-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture classicV3FuzzFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Body == "" || fixture.Open.Body == "" || fixture.Private.Body == "" {
		t.Fatal("Classic v3 fixture is missing a fuzz seed")
	}
	return fixture
}

func boundedClassicV3PublishError(code metaserver.ClassicPublishErrorCode) bool {
	switch code {
	case metaserver.ClassicPublishInvalidJSON,
		metaserver.ClassicPublishNonCanonicalJSON,
		metaserver.ClassicPublishUnsupportedSchema,
		metaserver.ClassicPublishBodyTooLarge,
		metaserver.ClassicPublishInvalidIdentity,
		metaserver.ClassicPublishInvalidCertificate,
		metaserver.ClassicPublishInvalidText,
		metaserver.ClassicPublishInvalidPlayers,
		metaserver.ClassicPublishInvalidEndpoint:
		return true
	default:
		return false
	}
}
