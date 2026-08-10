// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metaserverv1 "github.com/atrinik/protocol/gen/go/atrinik/metaserver/v1"
	"github.com/atrinik/protocol/metaserver"
)

type directoryFixtureManifest struct {
	FixtureVersion int    `json:"fixture_version"`
	Schema         string `json:"schema"`
	Positive       struct {
		JSON                string `json:"json"`
		XML                 string `json:"xml"`
		ProjectionSemantics string `json:"projection_semantics"`
		BodySHA256          string `json:"body_sha256"`
		HTTPStrongETag      string `json:"http_strong_etag"`
	} `json:"positive"`
	Negative []struct {
		File  string `json:"file"`
		Error string `json:"error"`
	} `json:"negative"`
	MaximumBounds struct {
		BodyBytes               int `json:"body_bytes"`
		Servers                 int `json:"servers"`
		NameUTF8Bytes           int `json:"name_utf8_bytes"`
		DescriptionUTF8Bytes    int `json:"description_utf8_bytes"`
		RegionBytes             int `json:"region_bytes"`
		ContentIDBytes          int `json:"content_id_bytes"`
		Players                 int `json:"players"`
		HostnameBytes           int `json:"hostname_bytes"`
		Port                    int `json:"port"`
		SnapshotLifetimeSeconds int `json:"snapshot_lifetime_seconds"`
		HTTPETagBytes           int `json:"http_etag_bytes"`
	} `json:"maximum_bounds"`
}

type directoryProjectionFixture struct {
	Schema      string   `json:"schema"`
	Generation  string   `json:"generation"`
	GeneratedAt string   `json:"generated_at"`
	ExpiresAt   string   `json:"expires_at"`
	ServerOrder []string `json:"server_order"`
	TextValues  []struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Region      *string `json:"region"`
	} `json:"text_values"`
	EndpointPresence []bool `json:"endpoint_presence"`
	PasswordRequired []bool `json:"password_required"`
	HTML             struct {
		AllDirectoryStringsAreText       bool `json:"all_directory_strings_are_text"`
		ActiveContentFromDirectoryValues bool `json:"active_content_from_directory_values"`
		AbsentValuesAreNotSynthesized    bool `json:"absent_values_are_not_synthesized"`
	} `json:"html"`
	XML struct {
		Fixture                string `json:"fixture"`
		AbsentValuesAreOmitted bool   `json:"absent_values_are_omitted"`
	} `json:"xml"`
}

func TestDirectoryLanguageNeutralFixtures(t *testing.T) {
	manifest := loadDirectoryManifest(t)
	if manifest.FixtureVersion != 2 || manifest.Schema != metaserver.DirectorySchema {
		t.Fatal("fixture manifest does not describe directory v1")
	}
	canonical := readDirectoryFixture(t, manifest.Positive.JSON)
	snapshot, err := metaserver.ParseDirectoryJSON(canonical)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := metaserver.MarshalDirectoryJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rendered, canonical) {
		t.Fatal("canonical JSON fixture did not round-trip byte-identically")
	}
	digest, err := metaserver.DirectoryJSONSHA256(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if digest != manifest.Positive.BodySHA256 {
		t.Fatalf("body SHA-256 %q differs from fixture %q", digest, manifest.Positive.BodySHA256)
	}
	if !metaserver.ValidDirectoryStrongETag(manifest.Positive.HTTPStrongETag) {
		t.Fatal("fixture HTTP ETag is not a valid opaque strong validator")
	}
	if len(snapshot.Servers) != 2 || snapshot.Servers[0].Region == nil ||
		snapshot.Servers[0].Endpoint == nil || snapshot.Servers[1].Region != nil ||
		snapshot.Servers[1].Endpoint != nil {
		t.Fatal("positive fixture presence semantics changed")
	}
	if snapshot.Servers[0].Endpoint.Hostname != "xn--bcher-kva.example.org" {
		t.Fatal("positive fixture no longer exercises a canonical IDNA A-label")
	}
	if len(readDirectoryFixture(t, manifest.Positive.XML)) == 0 {
		t.Fatal("XML semantic projection fixture is empty")
	}
	assertDirectoryProjection(t, manifest, snapshot)

	for _, fixture := range manifest.Negative {
		t.Run(filepath.Base(fixture.File), func(t *testing.T) {
			parsed, err := metaserver.ParseDirectoryJSON(readDirectoryFixture(t, fixture.File))
			if parsed != nil {
				t.Fatal("negative fixture returned a partial snapshot")
			}
			code, ok := metaserver.DirectoryErrorCodeOf(err)
			if !ok || string(code) != fixture.Error {
				t.Fatalf("negative fixture returned %v, want %s", err, fixture.Error)
			}
		})
	}
}

func TestDirectoryStrongETag(t *testing.T) {
	maximum := `"` + strings.Repeat("a", metaserver.MaximumDirectoryETagBytes-2) + `"`
	for _, value := range []string{`"r2-object-validator"`, `"0123456789abcdef"`, maximum} {
		if !metaserver.ValidDirectoryStrongETag(value) {
			t.Fatalf("valid strong ETag rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", `W/"weak"`, `""`, `"unterminated`, `unquoted`, "\"line\nbreak\"",
		`"back\\slash"`, `"embedded"quote"`,
		`"` + strings.Repeat("a", metaserver.MaximumDirectoryETagBytes-1) + `"`,
	} {
		if metaserver.ValidDirectoryStrongETag(value) {
			t.Fatalf("invalid strong ETag accepted: %q", value)
		}
	}
}

func TestDirectoryMaximumBounds(t *testing.T) {
	manifest := loadDirectoryManifest(t)
	assertDirectoryLimitsMatch(t, manifest)

	maximumHostname := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 61),
	}, ".")
	maximum := validDirectorySnapshot()
	maximum.GeneratedAtUnixSeconds = metaserver.MaximumDirectoryUnixSeconds -
		metaserver.MaximumDirectoryLifetimeSeconds
	maximum.ExpiresAtUnixSeconds = metaserver.MaximumDirectoryUnixSeconds
	maximum.Servers[0].Name = strings.Repeat("n", metaserver.MaximumDirectoryNameBytes)
	maximum.Servers[0].Description = strings.Repeat("d", metaserver.MaximumDirectoryDescriptionBytes)
	region := strings.Repeat("r", metaserver.MaximumDirectoryRegionBytes)
	maximum.Servers[0].Region = &region
	maximum.Servers[0].ContentId = strings.Repeat("c", metaserver.MaximumDirectoryContentIDBytes)
	maximum.Servers[0].PlayersOnline = metaserver.MaximumDirectoryPlayers
	maximum.Servers[0].PlayersCapacity = metaserver.MaximumDirectoryPlayers
	maximum.Servers[0].Status = metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL
	maximum.Servers[0].Endpoint = &metaserverv1.DirectEndpoint{
		Hostname: maximumHostname,
		Port:     65_535,
	}
	rendered, err := metaserver.MarshalDirectoryJSON(maximum)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metaserver.ParseDirectoryJSON(rendered); err != nil {
		t.Fatalf("maximum field fixture did not parse: %v", err)
	}

	maximumCount := validDirectorySnapshot()
	maximumCount.Servers = make([]*metaserverv1.DirectoryServer, 0, metaserver.MaximumDirectoryServers)
	for index := 0; index < metaserver.MaximumDirectoryServers; index++ {
		server := validDirectoryServer()
		server.ServerId = make([]byte, 32)
		binary.BigEndian.PutUint32(server.ServerId[28:], uint32(index+1))
		server.CertificateSha256 = bytes.Clone(server.ServerId)
		maximumCount.Servers = append(maximumCount.Servers, server)
	}
	rendered, err = metaserver.MarshalDirectoryJSON(maximumCount)
	if err != nil {
		t.Fatalf("maximum server-count fixture did not render: %v", err)
	}
	if len(rendered) > metaserver.MaximumDirectoryBodyBytes {
		t.Fatal("maximum server-count fixture exceeded the body limit")
	}
	if _, err := metaserver.ParseDirectoryJSON(rendered); err != nil {
		t.Fatalf("maximum server-count fixture did not parse: %v", err)
	}

	tooMany := validDirectorySnapshot()
	tooMany.Servers = make([]*metaserverv1.DirectoryServer, metaserver.MaximumDirectoryServers+1)
	if code := directoryErrorCode(t, metaserver.ValidateDirectory(tooMany)); code != metaserver.DirectoryTooManyServers {
		t.Fatalf("too many servers returned %s", code)
	}
	oversized := bytes.Repeat([]byte{'x'}, metaserver.MaximumDirectoryBodyBytes+1)
	if code := directoryErrorCode(t, parseDirectoryError(oversized)); code != metaserver.DirectoryBodyTooLarge {
		t.Fatalf("oversized body returned %s", code)
	}
}

func TestDirectoryFilteringAndFreshness(t *testing.T) {
	snapshot := validDirectorySnapshot()
	server := snapshot.Servers[0]
	if !metaserver.DirectoryFreshAt(snapshot, snapshot.GeneratedAtUnixSeconds) ||
		metaserver.DirectoryFreshAt(snapshot, snapshot.ExpiresAtUnixSeconds) {
		t.Fatal("snapshot freshness boundary changed")
	}
	future := validDirectorySnapshot()
	future.GeneratedAtUnixSeconds = 1_000
	future.ExpiresAtUnixSeconds = 2_000
	if !metaserver.DirectoryFreshAt(future, 700) || metaserver.DirectoryFreshAt(future, 699) {
		t.Fatal("future-skew boundary changed")
	}
	if !metaserver.DirectoryServerCompatible(
		server,
		1,
		0,
		"atrinik-main",
		server.ContentRevisionSha256,
	) {
		t.Fatal("exact compatible server was filtered")
	}
	if metaserver.DirectoryServerCompatible(
		server,
		1,
		1,
		"atrinik-main",
		server.ContentRevisionSha256,
	) {
		t.Fatal("wrong protocol minor was accepted")
	}
	if metaserver.DirectoryServerCompatible(server, 1, 0, "other", server.ContentRevisionSha256) {
		t.Fatal("wrong content identity was accepted")
	}
}

func TestDirectoryRejectsUnsafeBoundaries(t *testing.T) {
	oneByteIdentifiers := validDirectorySnapshot()
	region := "x"
	oneByteIdentifiers.Servers[0].Region = &region
	oneByteIdentifiers.Servers[0].ContentId = "a"
	if err := metaserver.ValidateDirectory(oneByteIdentifiers); err != nil {
		t.Fatalf("one-byte bounded identifiers were rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*metaserverv1.DirectorySnapshot)
		code   metaserver.DirectoryErrorCode
	}{
		{"identity split", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].CertificateSha256[0] ^= 1 }, metaserver.DirectoryInvalidIdentity},
		{"control text", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].Name = "unsafe\nname" }, metaserver.DirectoryInvalidText},
		{"line separator", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].Description = "unsafe\u2028text" }, metaserver.DirectoryInvalidText},
		{"paragraph separator", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].Description = "unsafe\u2029text" }, metaserver.DirectoryInvalidText},
		{"noncharacter fffe", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].Description = "unsafe\ufffetext" }, metaserver.DirectoryInvalidText},
		{"noncharacter ffff", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].Description = "unsafe\ufffftext" }, metaserver.DirectoryInvalidText},
		{"uppercase region", func(value *metaserverv1.DirectorySnapshot) { region := "EU"; value.Servers[0].Region = &region }, metaserver.DirectoryInvalidRegion},
		{"wrong protocol", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].ProtocolMajor = 2 }, metaserver.DirectoryInvalidProtocol},
		{"path content", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].ContentId = "../private" }, metaserver.DirectoryInvalidContent},
		{"players overflow", func(value *metaserverv1.DirectorySnapshot) { value.Servers[0].PlayersOnline = 2 }, metaserver.DirectoryInvalidPlayers},
		{"status disagreement", func(value *metaserverv1.DirectorySnapshot) {
			value.Servers[0].Status = metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL
		}, metaserver.DirectoryInvalidStatus},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validDirectorySnapshot()
			test.mutate(value)
			if code := directoryErrorCode(t, metaserver.ValidateDirectory(value)); code != test.code {
				t.Fatalf("invalid model returned %s, want %s", code, test.code)
			}
		})
	}

	for _, hostname := range []string{
		"192.0.2.1",
		"127.1",
		"0177.0.0.1",
		"0x7f.0.0.1",
		"0x7f.0x0.0x0.0x1",
		"2130706433",
		"[2001:db8::1]",
		"2001:db8::1",
		"localhost",
		"Example.org",
		"example.org.",
		"xn--a.example.org",
		"xn--0.example.org",
		"xn--0ca24w.example.org",
	} {
		t.Run(hostname, func(t *testing.T) {
			value := validDirectorySnapshot()
			value.Servers[0].Endpoint.Hostname = hostname
			if code := directoryErrorCode(t, metaserver.ValidateDirectory(value)); code != metaserver.DirectoryInvalidEndpoint {
				t.Fatalf("unsafe hostname returned %s", code)
			}
		})
	}
}

func FuzzDirectoryJSON(f *testing.F) {
	manifest := loadDirectoryManifest(f)
	f.Add(readDirectoryFixture(f, manifest.Positive.JSON))
	for _, fixture := range manifest.Negative {
		f.Add(readDirectoryFixture(f, fixture.File))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		snapshot, err := metaserver.ParseDirectoryJSON(input)
		if err != nil {
			if snapshot != nil {
				t.Fatal("failed parse returned a partial snapshot")
			}
			return
		}
		rendered, renderErr := metaserver.MarshalDirectoryJSON(snapshot)
		if renderErr != nil || !bytes.Equal(rendered, input) {
			t.Fatal("successful parse was not canonical and deterministic")
		}
	})
}

func validDirectorySnapshot() *metaserverv1.DirectorySnapshot {
	return &metaserverv1.DirectorySnapshot{
		Schema:                 metaserver.DirectorySchema,
		Generation:             1,
		GeneratedAtUnixSeconds: 1,
		ExpiresAtUnixSeconds:   2,
		Servers:                []*metaserverv1.DirectoryServer{validDirectoryServer()},
	}
}

func validDirectoryServer() *metaserverv1.DirectoryServer {
	serverID := bytes.Repeat([]byte{1}, 32)
	return &metaserverv1.DirectoryServer{
		ServerId:              serverID,
		CertificateSha256:     bytes.Clone(serverID),
		Name:                  "Server",
		Description:           "",
		ProtocolMajor:         1,
		ProtocolMinor:         0,
		ContentId:             "atrinik-main",
		ContentRevisionSha256: bytes.Repeat([]byte{2}, 32),
		PlayersOnline:         0,
		PlayersCapacity:       1,
		Status:                metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_ONLINE,
		Endpoint: &metaserverv1.DirectEndpoint{
			Hostname: "server.example.org",
			Port:     13_327,
		},
	}
}

func loadDirectoryManifest(t testing.TB) directoryFixtureManifest {
	t.Helper()
	encoded, err := os.ReadFile("../fixtures/metaserver-directory-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest directoryFixtureManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func readDirectoryFixture(t testing.TB, relative string) []byte {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("../fixtures", relative))
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func directoryErrorCode(t *testing.T, err error) metaserver.DirectoryErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("invalid value was accepted")
	}
	var directoryError *metaserver.DirectoryError
	if !errors.As(err, &directoryError) {
		t.Fatalf("unexpected error type: %v", err)
	}
	return directoryError.Code
}

func parseDirectoryError(input []byte) error {
	_, err := metaserver.ParseDirectoryJSON(input)
	return err
}

func assertDirectoryLimitsMatch(t *testing.T, manifest directoryFixtureManifest) {
	t.Helper()
	bounds := manifest.MaximumBounds
	if bounds.BodyBytes != metaserver.MaximumDirectoryBodyBytes ||
		bounds.Servers != metaserver.MaximumDirectoryServers ||
		bounds.NameUTF8Bytes != metaserver.MaximumDirectoryNameBytes ||
		bounds.DescriptionUTF8Bytes != metaserver.MaximumDirectoryDescriptionBytes ||
		bounds.RegionBytes != metaserver.MaximumDirectoryRegionBytes ||
		bounds.ContentIDBytes != metaserver.MaximumDirectoryContentIDBytes ||
		bounds.Players != metaserver.MaximumDirectoryPlayers ||
		bounds.HostnameBytes != 253 || bounds.Port != 65_535 ||
		bounds.SnapshotLifetimeSeconds != metaserver.MaximumDirectoryLifetimeSeconds ||
		bounds.HTTPETagBytes != metaserver.MaximumDirectoryETagBytes {
		t.Fatal("fixture maximum bounds differ from the Go contract")
	}
}

func assertDirectoryProjection(
	t *testing.T,
	manifest directoryFixtureManifest,
	snapshot *metaserverv1.DirectorySnapshot,
) {
	t.Helper()
	encoded := readDirectoryFixture(t, manifest.Positive.ProjectionSemantics)
	var projection directoryProjectionFixture
	if err := json.Unmarshal(encoded, &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Schema != snapshot.Schema || projection.Generation != "42" ||
		projection.GeneratedAt != "1786219200" || projection.ExpiresAt != "1786233600" ||
		len(projection.ServerOrder) != len(snapshot.Servers) ||
		len(projection.TextValues) != len(snapshot.Servers) ||
		len(projection.EndpointPresence) != len(snapshot.Servers) ||
		len(projection.PasswordRequired) != len(snapshot.Servers) {
		t.Fatal("projection metadata differs from the canonical fixture")
	}
	for index, server := range snapshot.Servers {
		text := projection.TextValues[index]
		if projection.ServerOrder[index] != hex.EncodeToString(server.ServerId) ||
			text.Name != server.Name || text.Description != server.Description ||
			!equalOptionalString(text.Region, server.Region) ||
			projection.EndpointPresence[index] != (server.Endpoint != nil) ||
			projection.PasswordRequired[index] != server.PasswordRequired {
			t.Fatalf("projection server %d differs from the canonical fixture", index)
		}
	}
	if !projection.HTML.AllDirectoryStringsAreText ||
		projection.HTML.ActiveContentFromDirectoryValues ||
		!projection.HTML.AbsentValuesAreNotSynthesized ||
		!projection.XML.AbsentValuesAreOmitted ||
		projection.XML.Fixture != filepath.Base(manifest.Positive.XML) {
		t.Fatal("projection safety requirements changed")
	}
}

func equalOptionalString(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
