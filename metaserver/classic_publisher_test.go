// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/atrinik/protocol/metaserver"
)

type classicV2PositiveFixture struct {
	Name            string                    `json:"name"`
	Sequence        string                    `json:"sequence"`
	Nonce           string                    `json:"nonce"`
	Created         int64                     `json:"created"`
	Expires         int64                     `json:"expires"`
	Body            string                    `json:"body"`
	ContentDigest   string                    `json:"content_digest"`
	Path            string                    `json:"path"`
	SignatureInput  string                    `json:"signature_input"`
	SignatureBase   string                    `json:"signature_base"`
	SignatureBase64 string                    `json:"signature_base64"`
	SignatureHeader string                    `json:"signature_header"`
	ExpectedState   classicV2PublicationState `json:"expected_state"`
}

type classicV2PublicationState struct {
	AuthenticatedPresence        bool   `json:"authenticated_presence"`
	PublicListing                bool   `json:"public_listing"`
	PublicFieldsRetained         bool   `json:"public_fields_retained"`
	Rendezvous                   string `json:"rendezvous"`
	PublicAuthorizationActive    bool   `json:"public_authorization_active"`
	PrivateServerControlRetained bool   `json:"private_server_control_retained"`
}

type classicV2NegativeFixture struct {
	Name  string                             `json:"name"`
	Body  string                             `json:"body"`
	Error metaserver.ClassicPublishErrorCode `json:"error"`
}

type classicV2SignatureNegativeFixture struct {
	Name                 string `json:"name"`
	FailureClass         string `json:"failure_class"`
	Positive             string `json:"positive"`
	CertificateDERBase64 string `json:"certificate_der_base64"`
	ServerID             string `json:"server_id"`
	Body                 string `json:"body"`
	SignatureBase        string `json:"signature_base"`
	SignatureBase64      string `json:"signature_base64"`
	Error                string `json:"error"`
}

type classicV2CrossProfileReplayFixture struct {
	Name                 string `json:"name"`
	SourceProfile        string `json:"source_profile"`
	TargetProfile        string `json:"target_profile"`
	ServerID             string `json:"server_id"`
	CertificateDERBase64 string `json:"certificate_der_base64"`
	Body                 string `json:"body"`
	SourceSignatureBase  string `json:"source_signature_base"`
	TargetSignatureBase  string `json:"target_signature_base"`
	SignatureBase64      string `json:"signature_base64"`
	TargetBodyError      string `json:"target_body_error"`
	TargetSignatureError string `json:"target_signature_error"`
}

type classicV2MigrationState struct {
	Mode                          string   `json:"mode"`
	HighWaterSequence             string   `json:"high_water_sequence"`
	RetainedNonces                []string `json:"retained_nonces"`
	V1AuthenticatedPresence       bool     `json:"v1_authenticated_presence"`
	V1ListingPresent              bool     `json:"v1_listing_present"`
	V1RendezvousGenerationPresent bool     `json:"v1_rendezvous_generation_present"`
	V1ControlsActive              bool     `json:"v1_controls_active"`
	PendingAuthorizations         int      `json:"pending_authorizations"`
	Tickets                       int      `json:"tickets"`
	V2Usable                      bool     `json:"v2_usable"`
}

type classicV2MigrationRequest struct {
	Profile  string `json:"profile"`
	Sequence string `json:"sequence"`
	Nonce    string `json:"nonce"`
}

type classicV2MigrationExpected struct {
	Accepted            bool                    `json:"accepted"`
	Error               string                  `json:"error"`
	MinimumNextSequence string                  `json:"minimum_next_sequence"`
	State               classicV2MigrationState `json:"state"`
}

type classicV2MigrationCase struct {
	Name     string                     `json:"name"`
	Before   classicV2MigrationState    `json:"before"`
	Request  classicV2MigrationRequest  `json:"request"`
	Expected classicV2MigrationExpected `json:"expected"`
}

type classicV2GlobalRetirementState struct {
	Mode                    string `json:"mode"`
	InFlightV1Commits       int    `json:"in_flight_v1_commits"`
	V1AuthenticatedPresence int    `json:"v1_authenticated_presences"`
	V1Listings              int    `json:"v1_listings"`
	V1RendezvousGenerations int    `json:"v1_rendezvous_generations"`
	V1Controls              int    `json:"v1_controls"`
	PendingAuthorizations   int    `json:"pending_authorizations"`
	Tickets                 int    `json:"tickets"`
	V2StateUnchanged        bool   `json:"v2_state_unchanged"`
	RollbackAllowed         bool   `json:"rollback_allowed"`
}

type classicV2GlobalRetirementFixture struct {
	Before     classicV2GlobalRetirementState `json:"before"`
	Activation struct {
		Kind         string `json:"kind"`
		Prerequisite string `json:"prerequisite"`
	} `json:"activation"`
	ExpectedAfter   classicV2GlobalRetirementState `json:"expected_after"`
	RejectedPublish struct {
		Profile                 string `json:"profile"`
		HTTPStatus              int    `json:"http_status"`
		CacheControl            string `json:"cache_control"`
		Body                    string `json:"body"`
		Inspection              string `json:"inspection"`
		SequenceOrNonceConsumed bool   `json:"sequence_or_nonce_consumed"`
		MinimumNextSequence     string `json:"minimum_next_sequence"`
	} `json:"rejected_publish"`
}

type classicV2Fixture struct {
	FixtureVersion       int    `json:"fixture_version"`
	Profile              string `json:"profile"`
	Authority            string `json:"authority"`
	Method               string `json:"method"`
	ContentType          string `json:"content_type"`
	ServerID             string `json:"server_id"`
	CertificateDERBase64 string `json:"certificate_der_base64"`
	MaximumBounds        struct {
		BodyBytes            int    `json:"body_bytes"`
		CertificateDERBytes  int    `json:"certificate_der_bytes"`
		NameUTF8Bytes        int    `json:"name_utf8_bytes"`
		PlayersCount         uint64 `json:"players_count"`
		VersionUTF8Bytes     int    `json:"version_utf8_bytes"`
		TextCommentUTF8Bytes int    `json:"text_comment_utf8_bytes"`
		HostnameBytes        int    `json:"hostname_bytes"`
		Port                 uint32 `json:"port"`
	} `json:"maximum_bounds"`
	Positive           []classicV2PositiveFixture           `json:"positive"`
	SignatureNegative  []classicV2SignatureNegativeFixture  `json:"signature_negative"`
	Negative           []classicV2NegativeFixture           `json:"negative"`
	CrossProfileReplay []classicV2CrossProfileReplayFixture `json:"cross_profile_replay"`
	Migration          struct {
		Cases []classicV2MigrationCase `json:"cases"`
	} `json:"migration"`
	GlobalV1Retirement classicV2GlobalRetirementFixture `json:"global_v1_retirement"`
}

func TestClassicV2PublisherSignatureNegativeFixtures(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	if len(fixture.SignatureNegative) != 15 {
		t.Fatal("signature-negative fixture does not cover every required failure class")
	}
	positives := make(map[string]classicV2PositiveFixture, len(fixture.Positive))
	for _, positive := range fixture.Positive {
		positives[positive.Name] = positive
	}
	seen := make(map[string]bool)
	for _, test := range fixture.SignatureNegative {
		t.Run(test.Name, func(t *testing.T) {
			positive, ok := positives[test.Positive]
			if !ok || test.FailureClass == "" || test.CertificateDERBase64 == "" || test.ServerID == "" ||
				test.Body == "" || test.SignatureBase == "" || test.SignatureBase64 == "" {
				t.Fatal("signature-negative fixture metadata is incomplete")
			}
			if test.FailureClass == "body_content_digest" {
				parameters := classicV2FixtureParameters(t, fixture, positive, metaserver.ClassicV2Profile)
				components, err := metaserver.Build(parameters, []byte(test.Body))
				if err != nil || components.SignatureBase != test.SignatureBase {
					t.Fatalf("mutated body does not construct the declared signature base: %v", err)
				}
			}
			certificate := decodeBase64(t, test.CertificateDERBase64)
			signature := decodeBase64(t, test.SignatureBase64)
			err := metaserver.VerifyCertificateSignature(
				certificate,
				test.ServerID,
				test.SignatureBase,
				signature,
			)
			switch test.Error {
			case "invalid_identity":
				if !errors.Is(err, metaserver.ErrInvalidIdentity) {
					t.Fatalf("identity failure returned %v", err)
				}
			case "invalid_signature":
				if !errors.Is(err, metaserver.ErrInvalidSignature) {
					t.Fatalf("signature failure returned %v", err)
				}
			default:
				t.Fatalf("unknown expected error %q", test.Error)
			}
			seen[test.FailureClass] = true
		})
	}
	for _, required := range []string{
		"body_content_digest", "authority", "sequence", "route_tag_domain", "route", "tag",
		"signed_server_id", "keyid", "certificate_fingerprint", "certificate_der",
		"certificate_key_type", "signature_length", "signature_coordinate",
	} {
		if !seen[required] {
			t.Fatalf("signature-negative fixture is missing %q", required)
		}
	}
}

func TestClassicV2PublisherGoldenFixtures(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	if fixture.FixtureVersion != 1 || fixture.Profile != "classic-v2" ||
		fixture.Method != "POST" || fixture.ContentType != metaserver.ContentType ||
		len(fixture.Positive) != 4 {
		t.Fatal("fixture metadata does not describe the complete Classic v2 matrix")
	}
	if fixture.MaximumBounds.BodyBytes != metaserver.MaximumBodyBytes ||
		fixture.MaximumBounds.CertificateDERBytes != metaserver.MaximumCertificateDERBytes ||
		fixture.MaximumBounds.NameUTF8Bytes != metaserver.MaximumDirectoryNameBytes ||
		fixture.MaximumBounds.PlayersCount != metaserver.MaximumClassicPlayers ||
		fixture.MaximumBounds.VersionUTF8Bytes != metaserver.MaximumClassicVersionBytes ||
		fixture.MaximumBounds.TextCommentUTF8Bytes != metaserver.MaximumClassicTextCommentBytes ||
		fixture.MaximumBounds.HostnameBytes != 253 || fixture.MaximumBounds.Port != 65_535 {
		t.Fatal("fixture bounds differ from the Classic v2 contract")
	}
	certificate := decodeBase64(t, fixture.CertificateDERBase64)
	seen := make(map[string]bool)
	for _, test := range fixture.Positive {
		t.Run(test.Name, func(t *testing.T) {
			request, err := metaserver.ParseClassicV2PublishJSON([]byte(test.Body))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := metaserver.MarshalClassicV2PublishJSON(request)
			if err != nil || string(encoded) != test.Body {
				t.Fatalf("fixture did not round-trip byte-identically: %v", err)
			}
			if hex.EncodeToString(request.ServerID) != fixture.ServerID ||
				base64.StdEncoding.EncodeToString(request.CertificateDER) != fixture.CertificateDERBase64 {
				t.Fatal("fixture identity and certificate do not match their metadata")
			}
			parameters := classicV2FixtureParameters(t, fixture, test, metaserver.ClassicV2Profile)
			components, err := metaserver.Build(parameters, encoded)
			if err != nil {
				t.Fatal(err)
			}
			if test.Expires != test.Created+metaserver.SignatureValidity ||
				components.Path != test.Path || components.ContentDigest != test.ContentDigest ||
				components.SignatureInput != test.SignatureInput ||
				components.SignatureBase != test.SignatureBase {
				t.Fatal("constructed Classic v2 signature components differ from the fixture")
			}
			signature := decodeBase64(t, test.SignatureBase64)
			if test.SignatureHeader != "atrinik=:"+test.SignatureBase64+":" {
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
			wantRendezvous := "not-found"
			if request.Public {
				wantRendezvous = "unauthenticated"
				if request.AccessCodeRequired {
					wantRendezvous = "proof-required"
				}
			}
			wantState := classicV2PublicationState{
				AuthenticatedPresence:        true,
				PublicListing:                request.Public,
				PublicFieldsRetained:         request.Public,
				Rendezvous:                   wantRendezvous,
				PublicAuthorizationActive:    request.Public && request.AccessCodeRequired,
				PrivateServerControlRetained: !request.Public,
			}
			if test.ExpectedState != wantState {
				t.Fatalf("publication state %#v, want %#v", test.ExpectedState, wantState)
			}

			for _, profile := range []metaserver.Profile{
				metaserver.ClassicV1Profile,
				metaserver.GameProfile,
			} {
				parameters.Profile = profile
				crossed, err := metaserver.Build(parameters, encoded)
				if err != nil {
					t.Fatal(err)
				}
				if err := metaserver.VerifyCertificateSignature(
					certificate,
					fixture.ServerID,
					crossed.SignatureBase,
					signature,
				); !errors.Is(err, metaserver.ErrInvalidSignature) {
					t.Fatalf("Classic v2 signature crossed profile %d: %v", profile, err)
				}
			}

			key := strconv.FormatBool(request.Public) + "/" +
				strconv.FormatBool(request.AccessCodeRequired) + "/" +
				strconv.FormatBool(request.Endpoint != nil)
			seen[key] = true
		})
	}
	for _, required := range []string{
		"true/false/false",
		"true/true/true",
		"false/false/true",
		"false/true/false",
	} {
		if !seen[required] {
			t.Fatalf("positive fixture matrix is missing %s", required)
		}
	}
}

func TestClassicV2CrossProfileReplayFixtures(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	if len(fixture.CrossProfileReplay) != 2 {
		t.Fatalf("cross-profile replay count %d, want 2", len(fixture.CrossProfileReplay))
	}
	v1 := loadPublisherFixture(t)
	seen := make(map[string]bool)
	for _, test := range fixture.CrossProfileReplay {
		t.Run(test.Name, func(t *testing.T) {
			if test.Name != strings.TrimPrefix(test.SourceProfile, "classic-")+"-at-"+strings.TrimPrefix(test.TargetProfile, "classic-") ||
				test.TargetBodyError != "unsupported_schema" ||
				test.TargetSignatureError != "invalid_signature" {
				t.Fatal("cross-profile replay metadata is incomplete")
			}
			certificate := decodeBase64(t, test.CertificateDERBase64)
			signature := decodeBase64(t, test.SignatureBase64)
			if err := metaserver.VerifyCertificateSignature(certificate, test.ServerID, test.SourceSignatureBase, signature); err != nil {
				t.Fatalf("source signature did not verify: %v", err)
			}
			if err := metaserver.VerifyCertificateSignature(certificate, test.ServerID, test.TargetSignatureBase, signature); !errors.Is(err, metaserver.ErrInvalidSignature) {
				t.Fatalf("cross-profile signature verified: %v", err)
			}

			var envelope struct {
				Schema string `json:"schema"`
			}
			if err := json.Unmarshal([]byte(test.Body), &envelope); err != nil {
				t.Fatal(err)
			}
			switch test.SourceProfile {
			case "classic-v1":
				if test.TargetProfile != "classic-v2" || test.Body != v1.Body || test.ServerID != v1.ServerID ||
					test.CertificateDERBase64 != v1.CertificateDERBase64 || test.SourceSignatureBase != v1.SignatureBase ||
					test.SignatureBase64 != v1.SignatureBase64 || envelope.Schema != "atrinik-classic-publish-v1" {
					t.Fatal("v1-at-v2 vector is not anchored to the frozen v1 golden")
				}
				if _, err := metaserver.ParseClassicV2PublishJSON([]byte(test.Body)); classicPublishErrorCode(t, err) != metaserver.ClassicPublishUnsupportedSchema {
					t.Fatalf("v1 body was not rejected by the v2 codec: %v", err)
				}
			case "classic-v2":
				if test.TargetProfile != "classic-v1" || envelope.Schema != "atrinik-classic-publish-v2" {
					t.Fatal("v2-at-v1 vector does not target the frozen v1 domain")
				}
				if _, err := metaserver.ParseClassicV2PublishJSON([]byte(test.Body)); err != nil {
					t.Fatalf("v2 replay source is not a valid v2 body: %v", err)
				}
				anchored := false
				for _, positive := range fixture.Positive {
					anchored = anchored || (test.Body == positive.Body && test.SourceSignatureBase == positive.SignatureBase && test.SignatureBase64 == positive.SignatureBase64)
				}
				if !anchored {
					t.Fatal("v2-at-v1 vector is not anchored to a v2 golden")
				}
			default:
				t.Fatalf("unknown source profile %q", test.SourceProfile)
			}
			seen[test.SourceProfile+"/"+test.TargetProfile] = true
		})
	}
	if !seen["classic-v1/classic-v2"] || !seen["classic-v2/classic-v1"] {
		t.Fatal("cross-profile replay fixture is not bidirectional")
	}
}

func TestClassicV2PublisherNegativeFixtures(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	if len(fixture.Negative) < 12 {
		t.Fatal("negative fixture does not cover the canonical contract")
	}
	seen := make(map[string]bool)
	for _, test := range fixture.Negative {
		t.Run(test.Name, func(t *testing.T) {
			request, err := metaserver.ParseClassicV2PublishJSON([]byte(test.Body))
			if request != nil {
				t.Fatal("rejected body returned a partial request")
			}
			code, ok := metaserver.ClassicPublishErrorCodeOf(err)
			if !ok || code != test.Error {
				t.Fatalf("rejected body returned (%q, %v), want %q", code, err, test.Error)
			}
			seen[test.Name] = true
		})
	}
	if !seen["endpoint-alias"] {
		t.Fatal("negative fixture does not cover an endpoint object alias")
	}
}

func TestClassicV2MigrationFixture(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	if len(fixture.Migration.Cases) != 8 {
		t.Fatalf("migration case count %d, want 8", len(fixture.Migration.Cases))
	}
	for _, test := range fixture.Migration.Cases {
		t.Run(test.Name, func(t *testing.T) {
			actual := applyClassicV2MigrationFixture(t, test.Before, test.Request)
			if !reflect.DeepEqual(actual, test.Expected) {
				t.Fatalf("transition result\n got: %#v\nwant: %#v", actual, test.Expected)
			}
			if !actual.Accepted && !reflect.DeepEqual(actual.State, test.Before) {
				t.Fatal("rejected migration mutated replay or publication state")
			}
		})
	}
}

func applyClassicV2MigrationFixture(t *testing.T, before classicV2MigrationState, request classicV2MigrationRequest) classicV2MigrationExpected {
	t.Helper()
	highWater, err := strconv.ParseUint(before.HighWaterSequence, 10, 64)
	if err != nil {
		t.Fatalf("invalid high-water sequence: %v", err)
	}
	sequence, err := strconv.ParseUint(request.Sequence, 10, 64)
	if err != nil {
		t.Fatalf("invalid request sequence: %v", err)
	}
	if request.Profile != "classic-v1" && request.Profile != "classic-v2" {
		t.Fatalf("unknown request profile %q", request.Profile)
	}
	if before.Mode != "classic-v1" && before.Mode != "classic-v2-only" {
		t.Fatalf("unknown migration mode %q", before.Mode)
	}
	for _, nonce := range append(append([]string(nil), before.RetainedNonces...), request.Nonce) {
		decoded, err := hex.DecodeString(nonce)
		if err != nil || len(decoded) != 16 {
			t.Fatalf("invalid retained or request nonce %q", nonce)
		}
	}

	result := classicV2MigrationExpected{
		State: before,
	}
	result.State.RetainedNonces = append([]string(nil), before.RetainedNonces...)
	if before.Mode == "classic-v2-only" && request.Profile == "classic-v1" {
		result.Error = "profile_retired"
		return result
	}
	if highWater == math.MaxUint64 {
		result.Error = "publish_sequence_exhausted"
		return result
	}
	result.MinimumNextSequence = strconv.FormatUint(highWater+1, 10)
	for _, nonce := range before.RetainedNonces {
		if nonce == request.Nonce {
			result.Error = "publish_replay"
			return result
		}
	}
	if sequence <= highWater {
		result.Error = "publish_replay"
		return result
	}

	result.Accepted = true
	if sequence != math.MaxUint64 {
		result.MinimumNextSequence = strconv.FormatUint(sequence+1, 10)
	} else {
		result.MinimumNextSequence = ""
	}
	result.State.HighWaterSequence = request.Sequence
	result.State.RetainedNonces = append(result.State.RetainedNonces, request.Nonce)
	if request.Profile == "classic-v2" {
		result.State.Mode = "classic-v2-only"
		result.State.V1AuthenticatedPresence = false
		result.State.V1ListingPresent = false
		result.State.V1RendezvousGenerationPresent = false
		result.State.V1ControlsActive = false
		result.State.PendingAuthorizations = 0
		result.State.Tickets = 0
		result.State.V2Usable = true
	}
	return result
}

func TestClassicV2GlobalRetirementFixture(t *testing.T) {
	gate := loadClassicV2Fixture(t).GlobalV1Retirement
	if gate.Before.Mode != "classic-v1-accepting" || gate.Before.InFlightV1Commits != 0 ||
		gate.Before.V1AuthenticatedPresence == 0 || gate.Before.V1Listings == 0 ||
		gate.Activation.Kind != "authorized-operator-transaction" ||
		gate.Activation.Prerequisite != "human-accepted-v5-canaries-and-one-way-alias-cutover" {
		t.Fatal("global retirement fixture lacks an explicit drained starting gate and human prerequisite")
	}
	after := gate.Before
	after.Mode = "classic-v1-retired"
	after.V1AuthenticatedPresence = 0
	after.V1Listings = 0
	after.V1RendezvousGenerations = 0
	after.V1Controls = 0
	after.PendingAuthorizations = 0
	after.Tickets = 0
	after.RollbackAllowed = false
	if after != gate.ExpectedAfter || !gate.ExpectedAfter.V2StateUnchanged {
		t.Fatalf("global retirement transition %#v, want %#v", gate.ExpectedAfter, after)
	}
	rejection := gate.RejectedPublish
	if rejection.Profile != "classic-v1" || rejection.HTTPStatus != 410 ||
		rejection.CacheControl != "no-store" || rejection.Body != `{"error":{"code":"profile_retired"}}` ||
		rejection.Inspection != "none" || rejection.SequenceOrNonceConsumed || rejection.MinimumNextSequence != "" {
		t.Fatal("global retirement rejection is not fixed, pre-inspection, and non-mutating")
	}
}

func TestClassicV2PublisherValidationClasses(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	base, err := metaserver.ParseClassicV2PublishJSON([]byte(fixture.Positive[1].Body))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*metaserver.ClassicV2PublishRequest)
		code   metaserver.ClassicPublishErrorCode
	}{
		{"identity", func(value *metaserver.ClassicV2PublishRequest) { value.ServerID[0] ^= 1 }, metaserver.ClassicPublishInvalidIdentity},
		{"name", func(value *metaserver.ClassicV2PublishRequest) { value.Name = "" }, metaserver.ClassicPublishInvalidText},
		{"name too long", func(value *metaserver.ClassicV2PublishRequest) { value.Name = strings.Repeat("n", 81) }, metaserver.ClassicPublishInvalidText},
		{"version", func(value *metaserver.ClassicV2PublishRequest) { value.Version = strings.Repeat("v", 33) }, metaserver.ClassicPublishInvalidText},
		{"comment", func(value *metaserver.ClassicV2PublishRequest) { value.TextComment = "bad\nvalue" }, metaserver.ClassicPublishInvalidText},
		{"comment too long", func(value *metaserver.ClassicV2PublishRequest) { value.TextComment = strings.Repeat("c", 257) }, metaserver.ClassicPublishInvalidText},
		{"endpoint", func(value *metaserver.ClassicV2PublishRequest) { value.Endpoint.Hostname = "192.0.2.1" }, metaserver.ClassicPublishInvalidEndpoint},
		{"certificate", func(value *metaserver.ClassicV2PublishRequest) {
			value.CertificateDER = []byte("not a certificate")
			digest := sha256.Sum256(value.CertificateDER)
			value.ServerID = append([]byte(nil), digest[:]...)
		}, metaserver.ClassicPublishInvalidCertificate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneClassicV2Request(base)
			test.mutate(value)
			err := metaserver.ValidateClassicV2Publish(value)
			code, ok := metaserver.ClassicPublishErrorCodeOf(err)
			if !ok || code != test.code {
				t.Fatalf("invalid model returned (%q, %v), want %q", code, err, test.code)
			}
		})
	}
	if code := classicPublishErrorCode(t, metaserver.ValidateClassicV2Publish(nil)); code != metaserver.ClassicPublishInvalidJSON {
		t.Fatalf("nil request returned %q", code)
	}
}

func TestClassicV2PublisherPreservesClassicTextScalars(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	request, err := metaserver.ParseClassicV2PublishJSON([]byte(fixture.Positive[0].Body))
	if err != nil {
		t.Fatal(err)
	}
	request.TextComment = "line\u2028paragraph\u2029C1:\u0085 noncharacters:\ufffe\uffff slash:\\u2028 combo:\\\u2028"
	encoded, err := metaserver.MarshalClassicV2PublishJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), `\u2028`) != 1 || strings.Contains(string(encoded), `\u2029`) {
		t.Fatal("Classic scalar values were emitted as noncanonical JSON escapes")
	}
	if !strings.Contains(string(encoded), `slash:\\u2028`) {
		t.Fatal("literal reverse-solidus text was changed while rendering scalars")
	}
	parsed, err := metaserver.ParseClassicV2PublishJSON(encoded)
	if err != nil || parsed.TextComment != request.TextComment {
		t.Fatalf("Classic scalar text did not round-trip: %v", err)
	}
}

func TestClassicV2PublisherMaximumModel(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	request, err := metaserver.ParseClassicV2PublishJSON([]byte(fixture.Positive[1].Body))
	if err != nil {
		t.Fatal(err)
	}
	request.Name = strings.Repeat("n", metaserver.MaximumDirectoryNameBytes)
	request.PlayersCount = ^uint32(0)
	request.Version = strings.Repeat("v", metaserver.MaximumClassicVersionBytes)
	request.TextComment = strings.Repeat("c", metaserver.MaximumClassicTextCommentBytes)
	request.Endpoint.Hostname = strings.Repeat("a", 63) + "." +
		strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	request.Endpoint.Port = 65_535
	encoded, err := metaserver.MarshalClassicV2PublishJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Endpoint.Hostname) != fixture.MaximumBounds.HostnameBytes {
		t.Fatal("maximum hostname fixture does not reach its byte bound")
	}
	if _, err := metaserver.ParseClassicV2PublishJSON(encoded); err != nil {
		t.Fatalf("maximum valid model did not round-trip: %v", err)
	}
}

func TestClassicV2PublisherInputBounds(t *testing.T) {
	fixture := loadClassicV2Fixture(t)
	tests := []struct {
		name string
		body []byte
		code metaserver.ClassicPublishErrorCode
	}{
		{"empty", nil, metaserver.ClassicPublishInvalidJSON},
		{"malformed", []byte(`{"schema":`), metaserver.ClassicPublishInvalidJSON},
		{"invalid UTF-8", []byte{'{', 0xff, '}'}, metaserver.ClassicPublishInvalidJSON},
		{"maximum plus one", []byte(strings.Repeat("x", metaserver.MaximumBodyBytes+1)), metaserver.ClassicPublishBodyTooLarge},
		{"trailing LF", []byte(fixture.Positive[0].Body + "\n"), metaserver.ClassicPublishNonCanonicalJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := metaserver.ParseClassicV2PublishJSON(test.body)
			if code := classicPublishErrorCode(t, err); code != test.code {
				t.Fatalf("invalid input returned %q, want %q", code, test.code)
			}
		})
	}
}

func FuzzClassicV2PublishJSON(f *testing.F) {
	fixture := loadClassicV2Fixture(f)
	for _, test := range fixture.Positive {
		f.Add([]byte(test.Body))
	}
	for _, test := range fixture.Negative {
		f.Add([]byte(test.Body))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		request, err := metaserver.ParseClassicV2PublishJSON(input)
		if err != nil {
			if _, ok := metaserver.ClassicPublishErrorCodeOf(err); !ok {
				t.Fatalf("unbounded error type: %T", err)
			}
			return
		}
		encoded, err := metaserver.MarshalClassicV2PublishJSON(request)
		if err != nil || string(encoded) != string(input) {
			t.Fatal("accepted body did not round-trip canonically")
		}
	})
}

func loadClassicV2Fixture(t testing.TB) classicV2Fixture {
	t.Helper()
	data, err := os.ReadFile("../fixtures/metaserver-classic-publisher-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture classicV2Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func classicV2FixtureParameters(
	t *testing.T,
	fixture classicV2Fixture,
	test classicV2PositiveFixture,
	profile metaserver.Profile,
) metaserver.Parameters {
	t.Helper()
	sequence, err := strconv.ParseUint(test.Sequence, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	nonceBytes, err := hex.DecodeString(test.Nonce)
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
		Created:   test.Created,
	}
}

func cloneClassicV2Request(value *metaserver.ClassicV2PublishRequest) *metaserver.ClassicV2PublishRequest {
	clone := *value
	clone.ServerID = append([]byte(nil), value.ServerID...)
	clone.CertificateDER = append([]byte(nil), value.CertificateDER...)
	if value.Endpoint != nil {
		endpoint := *value.Endpoint
		clone.Endpoint = &endpoint
	}
	return &clone
}

func classicPublishErrorCode(t *testing.T, err error) metaserver.ClassicPublishErrorCode {
	t.Helper()
	code, ok := metaserver.ClassicPublishErrorCodeOf(err)
	if !ok {
		t.Fatalf("unexpected error %v", err)
	}
	return code
}
