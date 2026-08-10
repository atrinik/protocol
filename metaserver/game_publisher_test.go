// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	metaserverv1 "github.com/atrinik/protocol/gen/go/atrinik/metaserver/v1"
	"github.com/atrinik/protocol/metaserver"
	"google.golang.org/protobuf/proto"
)

type gamePublisherNegativeFixture struct {
	Name  string                          `json:"name"`
	Body  string                          `json:"body"`
	Error metaserver.GamePublishErrorCode `json:"error"`
}

type gamePublisherFixture struct {
	Version              int                            `json:"version"`
	Profile              string                         `json:"profile"`
	Authority            string                         `json:"authority"`
	Method               string                         `json:"method"`
	ContentType          string                         `json:"content_type"`
	ServerID             string                         `json:"server_id"`
	CertificateDERBase64 string                         `json:"certificate_der_base64"`
	Path                 string                         `json:"path"`
	Sequence             string                         `json:"sequence"`
	Nonce                string                         `json:"nonce"`
	Created              int64                          `json:"created"`
	Expires              int64                          `json:"expires"`
	Body                 string                         `json:"body"`
	ContentDigest        string                         `json:"content_digest"`
	SignatureInput       string                         `json:"signature_input"`
	SignatureBase        string                         `json:"signature_base"`
	SignatureBase64      string                         `json:"signature_base64"`
	SignatureHeader      string                         `json:"signature_header"`
	SuccessBody          string                         `json:"success_body"`
	PrivateBody          string                         `json:"private_body"`
	MaximumBounds        gamePublisherMaximumBounds     `json:"maximum_bounds"`
	Negative             []gamePublisherNegativeFixture `json:"negative"`
}

type gamePublisherMaximumBounds struct {
	BodyBytes            int `json:"body_bytes"`
	CertificateDERBytes  int `json:"certificate_der_bytes"`
	NameUTF8Bytes        int `json:"name_utf8_bytes"`
	DescriptionUTF8Bytes int `json:"description_utf8_bytes"`
	RegionBytes          int `json:"region_bytes"`
	ContentIDBytes       int `json:"content_id_bytes"`
	Players              int `json:"players"`
	HostnameBytes        int `json:"hostname_bytes"`
	Port                 int `json:"port"`
}

func TestGamePublisherGoldenFixture(t *testing.T) {
	fixture := loadGamePublisherFixture(t)
	if fixture.Version != 1 || fixture.Profile != "game" || fixture.Method != "POST" ||
		fixture.ContentType != metaserver.ContentType || fixture.Expires != fixture.Created+300 ||
		fixture.SuccessBody != `{"status":"ok","rendezvousToken":"{64-lower-hex}"}` {
		t.Fatal("fixture metadata does not describe the v1 Game profile")
	}
	if fixture.MaximumBounds.BodyBytes != metaserver.MaximumBodyBytes ||
		fixture.MaximumBounds.CertificateDERBytes != metaserver.MaximumCertificateDERBytes ||
		fixture.MaximumBounds.NameUTF8Bytes != metaserver.MaximumDirectoryNameBytes ||
		fixture.MaximumBounds.DescriptionUTF8Bytes != metaserver.MaximumDirectoryDescriptionBytes ||
		fixture.MaximumBounds.RegionBytes != metaserver.MaximumDirectoryRegionBytes ||
		fixture.MaximumBounds.ContentIDBytes != metaserver.MaximumDirectoryContentIDBytes ||
		fixture.MaximumBounds.Players != metaserver.MaximumDirectoryPlayers ||
		fixture.MaximumBounds.HostnameBytes != 253 || fixture.MaximumBounds.Port != 65_535 {
		t.Fatal("fixture bounds differ from the shared Game directory contract")
	}

	request, err := metaserver.ParseGamePublishJSON([]byte(fixture.Body))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := metaserver.MarshalGamePublishJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != fixture.Body || hex.EncodeToString(request.Server.ServerId) != fixture.ServerID ||
		base64.StdEncoding.EncodeToString(request.CertificateDER) != fixture.CertificateDERBase64 ||
		!request.Public || request.Server.Region == nil || request.Server.Endpoint == nil {
		t.Fatal("positive fixture did not round-trip to the declared public model")
	}

	parameters := gameFixtureParameters(t, fixture, metaserver.GameProfile)
	components, err := metaserver.Build(parameters, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if components.Path != fixture.Path || components.ContentDigest != fixture.ContentDigest ||
		components.SignatureInput != fixture.SignatureInput ||
		components.SignatureBase != fixture.SignatureBase {
		t.Fatal("constructed Game signature components differ from the fixture")
	}
	certificate := decodeGameBase64(t, fixture.CertificateDERBase64)
	signature := decodeGameBase64(t, fixture.SignatureBase64)
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

	parameters.Profile = metaserver.ClassicProfile
	classic, err := metaserver.Build(parameters, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := metaserver.VerifyCertificateSignature(
		certificate,
		fixture.ServerID,
		classic.SignatureBase,
		signature,
	); !errors.Is(err, metaserver.ErrInvalidSignature) {
		t.Fatalf("Game signature crossed the classic domain: %v", err)
	}
}

func TestGamePublisherPrivateFixture(t *testing.T) {
	fixture := loadGamePublisherFixture(t)
	request, err := metaserver.ParseGamePublishJSON([]byte(fixture.PrivateBody))
	if err != nil {
		t.Fatal(err)
	}
	if request.Public || request.Server.Region != nil || request.Server.Endpoint != nil ||
		request.Server.Status != metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_MAINTENANCE ||
		!request.Server.PasswordRequired {
		t.Fatal("private fixture did not preserve its exact bounded model")
	}
	encoded, err := metaserver.MarshalGamePublishJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != fixture.PrivateBody {
		t.Fatal("private fixture was not byte-identical after round-trip")
	}
}

func TestGamePublisherNegativeFixture(t *testing.T) {
	fixture := loadGamePublisherFixture(t)
	if len(fixture.Negative) == 0 {
		t.Fatal("negative fixture is empty")
	}
	for _, test := range fixture.Negative {
		t.Run(test.Name, func(t *testing.T) {
			_, err := metaserver.ParseGamePublishJSON([]byte(test.Body))
			code, ok := metaserver.GamePublishErrorCodeOf(err)
			if !ok || code != test.Error {
				t.Fatalf("rejected body returned (%q, %v), want %q", code, err, test.Error)
			}
		})
	}
}

func TestGamePublisherValidationClasses(t *testing.T) {
	fixture := loadGamePublisherFixture(t)
	base, err := metaserver.ParseGamePublishJSON([]byte(fixture.Body))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*metaserver.GamePublishRequest)
		code   metaserver.GamePublishErrorCode
	}{
		{"identity", func(value *metaserver.GamePublishRequest) { value.Server.ServerId[0] ^= 1 }, metaserver.GamePublishInvalidIdentity},
		{"text", func(value *metaserver.GamePublishRequest) { value.Server.Name = "" }, metaserver.GamePublishInvalidText},
		{"region", func(value *metaserver.GamePublishRequest) { region := "EU"; value.Server.Region = &region }, metaserver.GamePublishInvalidRegion},
		{"protocol", func(value *metaserver.GamePublishRequest) { value.Server.ProtocolMajor = 2 }, metaserver.GamePublishInvalidProtocol},
		{"content", func(value *metaserver.GamePublishRequest) { value.Server.ContentId = "Upper" }, metaserver.GamePublishInvalidContent},
		{"players", func(value *metaserver.GamePublishRequest) {
			value.Server.PlayersOnline = value.Server.PlayersCapacity + 1
		}, metaserver.GamePublishInvalidPlayers},
		{"status", func(value *metaserver.GamePublishRequest) {
			value.Server.Status = metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL
		}, metaserver.GamePublishInvalidStatus},
		{"endpoint", func(value *metaserver.GamePublishRequest) { value.Server.Endpoint.Hostname = "127.0.0.1" }, metaserver.GamePublishInvalidEndpoint},
		{"certificate", func(value *metaserver.GamePublishRequest) {
			value.CertificateDER = []byte("not a certificate")
			digest := sha256.Sum256(value.CertificateDER)
			value.Server.ServerId = append([]byte(nil), digest[:]...)
			value.Server.CertificateSha256 = append([]byte(nil), digest[:]...)
		}, metaserver.GamePublishInvalidCertificate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := &metaserver.GamePublishRequest{
				CertificateDER: append([]byte(nil), base.CertificateDER...),
				Server:         proto.Clone(base.Server).(*metaserverv1.DirectoryServer),
				Public:         base.Public,
			}
			test.mutate(value)
			err := metaserver.ValidateGamePublish(value)
			code, ok := metaserver.GamePublishErrorCodeOf(err)
			if !ok || code != test.code {
				t.Fatalf("invalid model returned (%q, %v), want %q", code, err, test.code)
			}
		})
	}
	if err := metaserver.ValidateGamePublish(nil); gamePublishErrorCode(t, err) != metaserver.GamePublishInvalidJSON {
		t.Fatalf("nil request returned %v", err)
	}
}

func TestGamePublisherInputBounds(t *testing.T) {
	fixture := loadGamePublisherFixture(t)
	tests := []struct {
		name string
		body []byte
		code metaserver.GamePublishErrorCode
	}{
		{"empty", nil, metaserver.GamePublishInvalidJSON},
		{"malformed", []byte(`{"schema":`), metaserver.GamePublishInvalidJSON},
		{"invalid UTF-8", []byte{'{', 0xff, '}'}, metaserver.GamePublishInvalidJSON},
		{"maximum plus one", []byte(strings.Repeat("x", metaserver.MaximumBodyBytes+1)), metaserver.GamePublishBodyTooLarge},
		{"trailing LF", []byte(fixture.Body + "\n"), metaserver.GamePublishNonCanonicalJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := metaserver.ParseGamePublishJSON(test.body)
			if code := gamePublishErrorCode(t, err); code != test.code {
				t.Fatalf("invalid input returned %q, want %q", code, test.code)
			}
		})
	}
}

func FuzzGamePublishJSON(f *testing.F) {
	fixture := loadGamePublisherFixture(f)
	f.Add([]byte(fixture.Body))
	f.Add([]byte(fixture.PrivateBody))
	for _, test := range fixture.Negative {
		f.Add([]byte(test.Body))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		request, err := metaserver.ParseGamePublishJSON(input)
		if err != nil {
			if _, ok := metaserver.GamePublishErrorCodeOf(err); !ok {
				t.Fatalf("unbounded error type: %T", err)
			}
			return
		}
		encoded, err := metaserver.MarshalGamePublishJSON(request)
		if err != nil {
			t.Fatalf("accepted body failed to render: %v", err)
		}
		if string(encoded) != string(input) {
			t.Fatal("accepted body was not canonical")
		}
	})
}

func loadGamePublisherFixture(t testing.TB) gamePublisherFixture {
	t.Helper()
	data, err := os.ReadFile("../fixtures/metaserver-game-publisher-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture gamePublisherFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func gameFixtureParameters(
	t *testing.T,
	fixture gamePublisherFixture,
	profile metaserver.Profile,
) metaserver.Parameters {
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

func decodeGameBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func gamePublishErrorCode(t *testing.T, err error) metaserver.GamePublishErrorCode {
	t.Helper()
	code, ok := metaserver.GamePublishErrorCodeOf(err)
	if !ok {
		t.Fatalf("unexpected error %v", err)
	}
	return code
}
