// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	metaserverv1 "github.com/atrinik/protocol/gen/go/atrinik/metaserver/v1"
)

const GamePublishSchema = "atrinik-game-publish-v1"

const maximumCertificateBase64Bytes = ((MaximumCertificateDERBytes + 2) / 3) * 4

// GamePublishRequest is one authenticated GP1 publication body after strict
// canonical JSON validation. CertificateDER and Server remain caller-owned
// when passed to MarshalGamePublishJSON; ParseGamePublishJSON returns new
// allocations owned by the caller.
type GamePublishRequest struct {
	CertificateDER []byte
	Server         *metaserverv1.DirectoryServer
	Public         bool
}

// GamePublishErrorCode is a stable, bounded conformance failure class. It
// never contains rejected input.
type GamePublishErrorCode string

const (
	GamePublishInvalidJSON        GamePublishErrorCode = "invalid_json"
	GamePublishNonCanonicalJSON   GamePublishErrorCode = "noncanonical_json"
	GamePublishUnsupportedSchema  GamePublishErrorCode = "unsupported_schema"
	GamePublishBodyTooLarge       GamePublishErrorCode = "body_too_large"
	GamePublishInvalidIdentity    GamePublishErrorCode = "invalid_identity"
	GamePublishInvalidCertificate GamePublishErrorCode = "invalid_certificate"
	GamePublishInvalidText        GamePublishErrorCode = "invalid_text"
	GamePublishInvalidRegion      GamePublishErrorCode = "invalid_region"
	GamePublishInvalidProtocol    GamePublishErrorCode = "invalid_protocol"
	GamePublishInvalidContent     GamePublishErrorCode = "invalid_content"
	GamePublishInvalidPlayers     GamePublishErrorCode = "invalid_players"
	GamePublishInvalidStatus      GamePublishErrorCode = "invalid_status"
	GamePublishInvalidEndpoint    GamePublishErrorCode = "invalid_endpoint"
)

// GamePublishError reports only a stable class so malformed authenticated
// input cannot enter diagnostics.
type GamePublishError struct {
	Code GamePublishErrorCode
}

func (err *GamePublishError) Error() string {
	return "invalid Game Protocol 1 publication: " + string(err.Code)
}

// GamePublishErrorCodeOf returns a bounded error class and false for unrelated
// errors.
func GamePublishErrorCodeOf(err error) (GamePublishErrorCode, bool) {
	var publishError *GamePublishError
	if !errors.As(err, &publishError) {
		return "", false
	}
	return publishError.Code, true
}

type gamePublishWire struct {
	Schema           string        `json:"schema"`
	ServerID         string        `json:"serverId"`
	Certificate      string        `json:"certificate"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	Region           *string       `json:"region,omitempty"`
	Protocol         protocolWire  `json:"protocol"`
	Content          contentWire   `json:"content"`
	Players          playersWire   `json:"players"`
	Status           string        `json:"status"`
	Public           bool          `json:"public"`
	PasswordRequired bool          `json:"passwordRequired"`
	Endpoint         *endpointWire `json:"endpoint,omitempty"`
}

// ParseGamePublishJSON validates one complete canonical GP1 publisher body.
// Failure returns no partial model and does not mutate caller-owned state.
func ParseGamePublishJSON(input []byte) (*GamePublishRequest, error) {
	if len(input) > MaximumBodyBytes {
		return nil, gamePublishFailure(GamePublishBodyTooLarge)
	}
	if len(input) == 0 || !utf8.Valid(input) {
		return nil, gamePublishFailure(GamePublishInvalidJSON)
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	var wire gamePublishWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, gamePublishFailure(GamePublishInvalidJSON)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, gamePublishFailure(GamePublishNonCanonicalJSON)
	}

	request, err := gamePublishFromWire(wire)
	if err != nil {
		return nil, err
	}
	canonical, err := MarshalGamePublishJSON(request)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, input) {
		return nil, gamePublishFailure(GamePublishNonCanonicalJSON)
	}
	return request, nil
}

// MarshalGamePublishJSON validates and renders one canonical GP1 publisher
// body. The returned allocation has no trailing LF or insignificant bytes.
func MarshalGamePublishJSON(request *GamePublishRequest) ([]byte, error) {
	if err := ValidateGamePublish(request); err != nil {
		return nil, err
	}
	wire, err := gamePublishToWire(request)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wire); err != nil {
		return nil, gamePublishFailure(GamePublishInvalidJSON)
	}
	body := encoded.Bytes()
	if len(body) == 0 || body[len(body)-1] != '\n' {
		return nil, gamePublishFailure(GamePublishInvalidJSON)
	}
	body = body[:len(body)-1]
	if len(body) == 0 || len(body) > MaximumBodyBytes {
		return nil, gamePublishFailure(GamePublishBodyTooLarge)
	}
	return append([]byte(nil), body...), nil
}

// ValidateGamePublish enforces semantic bounds, certificate identity, and the
// P-256 key requirement without consulting a clock or retaining references.
func ValidateGamePublish(request *GamePublishRequest) error {
	if request == nil || request.Server == nil {
		return gamePublishFailure(GamePublishInvalidJSON)
	}
	if err := validateDirectoryServer(request.Server); err != nil {
		return mapDirectoryServerError(err)
	}
	if len(request.CertificateDER) == 0 ||
		len(request.CertificateDER) > MaximumCertificateDERBytes {
		return gamePublishFailure(GamePublishInvalidCertificate)
	}
	fingerprint := sha256.Sum256(request.CertificateDER)
	if !bytes.Equal(fingerprint[:], request.Server.ServerId) {
		return gamePublishFailure(GamePublishInvalidIdentity)
	}
	certificate, err := x509.ParseCertificate(request.CertificateDER)
	if err != nil {
		return gamePublishFailure(GamePublishInvalidCertificate)
	}
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return gamePublishFailure(GamePublishInvalidCertificate)
	}
	return nil
}

func gamePublishFromWire(wire gamePublishWire) (*GamePublishRequest, error) {
	if wire.Schema != GamePublishSchema {
		return nil, gamePublishFailure(GamePublishUnsupportedSchema)
	}
	serverID, err := decodeDirectoryDigest(wire.ServerID, DirectoryInvalidIdentity)
	if err != nil {
		return nil, gamePublishFailure(GamePublishInvalidIdentity)
	}
	if wire.Certificate == "" || len(wire.Certificate) > maximumCertificateBase64Bytes {
		return nil, gamePublishFailure(GamePublishInvalidCertificate)
	}
	certificate, err := base64.StdEncoding.Strict().DecodeString(wire.Certificate)
	if err != nil {
		return nil, gamePublishFailure(GamePublishInvalidCertificate)
	}
	contentRevision, err := decodeDirectoryDigest(
		wire.Content.RevisionSHA256,
		DirectoryInvalidContent,
	)
	if err != nil {
		return nil, gamePublishFailure(GamePublishInvalidContent)
	}
	status, ok := directoryStatusFromWire(wire.Status)
	if !ok {
		return nil, gamePublishFailure(GamePublishInvalidStatus)
	}
	server := &metaserverv1.DirectoryServer{
		ServerId:              serverID,
		CertificateSha256:     append([]byte(nil), serverID...),
		Name:                  wire.Name,
		Description:           wire.Description,
		Region:                wire.Region,
		ProtocolMajor:         wire.Protocol.Major,
		ProtocolMinor:         wire.Protocol.Minor,
		ContentId:             wire.Content.ID,
		ContentRevisionSha256: contentRevision,
		PlayersOnline:         wire.Players.Online,
		PlayersCapacity:       wire.Players.Capacity,
		Status:                status,
		PasswordRequired:      wire.PasswordRequired,
	}
	if wire.Endpoint != nil {
		server.Endpoint = &metaserverv1.DirectEndpoint{
			Hostname: wire.Endpoint.Hostname,
			Port:     wire.Endpoint.Port,
		}
	}
	request := &GamePublishRequest{
		CertificateDER: certificate,
		Server:         server,
		Public:         wire.Public,
	}
	if err := ValidateGamePublish(request); err != nil {
		return nil, err
	}
	return request, nil
}

func gamePublishToWire(request *GamePublishRequest) (gamePublishWire, error) {
	status, ok := directoryStatusToWire(request.Server.Status)
	if !ok {
		return gamePublishWire{}, gamePublishFailure(GamePublishInvalidStatus)
	}
	wire := gamePublishWire{
		Schema:      GamePublishSchema,
		ServerID:    hex.EncodeToString(request.Server.ServerId),
		Certificate: base64.StdEncoding.EncodeToString(request.CertificateDER),
		Name:        request.Server.Name,
		Description: request.Server.Description,
		Region:      request.Server.Region,
		Protocol: protocolWire{
			Major: request.Server.ProtocolMajor,
			Minor: request.Server.ProtocolMinor,
		},
		Content: contentWire{
			ID:             request.Server.ContentId,
			RevisionSHA256: hex.EncodeToString(request.Server.ContentRevisionSha256),
		},
		Players: playersWire{
			Online:   request.Server.PlayersOnline,
			Capacity: request.Server.PlayersCapacity,
		},
		Status:           status,
		Public:           request.Public,
		PasswordRequired: request.Server.PasswordRequired,
	}
	if request.Server.Endpoint != nil {
		wire.Endpoint = &endpointWire{
			Hostname: request.Server.Endpoint.Hostname,
			Port:     request.Server.Endpoint.Port,
		}
	}
	return wire, nil
}

func mapDirectoryServerError(err error) error {
	code, ok := DirectoryErrorCodeOf(err)
	if !ok {
		return gamePublishFailure(GamePublishInvalidJSON)
	}
	switch code {
	case DirectoryInvalidIdentity:
		return gamePublishFailure(GamePublishInvalidIdentity)
	case DirectoryInvalidText:
		return gamePublishFailure(GamePublishInvalidText)
	case DirectoryInvalidRegion:
		return gamePublishFailure(GamePublishInvalidRegion)
	case DirectoryInvalidProtocol:
		return gamePublishFailure(GamePublishInvalidProtocol)
	case DirectoryInvalidContent:
		return gamePublishFailure(GamePublishInvalidContent)
	case DirectoryInvalidPlayers:
		return gamePublishFailure(GamePublishInvalidPlayers)
	case DirectoryInvalidStatus:
		return gamePublishFailure(GamePublishInvalidStatus)
	case DirectoryInvalidEndpoint:
		return gamePublishFailure(GamePublishInvalidEndpoint)
	default:
		return gamePublishFailure(GamePublishInvalidJSON)
	}
}

func gamePublishFailure(code GamePublishErrorCode) error {
	return &GamePublishError{Code: code}
}
