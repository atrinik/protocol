// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	metaserverv1 "github.com/atrinik/protocol/gen/go/atrinik/metaserver/v1"
	"golang.org/x/net/idna"
)

const (
	DirectorySchema                  = "atrinik-directory-v1"
	MaximumDirectoryBodyBytes        = 262_144
	MaximumDirectoryServers          = 512
	MaximumDirectoryLifetimeSeconds  = 14_400
	MaximumDirectoryFutureSkew       = 300
	MaximumDirectoryUnixSeconds      = 253_402_300_799
	MaximumDirectoryNameBytes        = 80
	MaximumDirectoryDescriptionBytes = 512
	MaximumDirectoryRegionBytes      = 32
	MaximumDirectoryContentIDBytes   = 64
	MaximumDirectoryPlayers          = 100_000
)

var directoryIDNAProfile = idna.New(
	idna.MapForLookup(),
	idna.Transitional(false),
	idna.BidiRule(),
	idna.CheckHyphens(true),
	idna.CheckJoiners(true),
	idna.StrictDomainName(true),
	idna.VerifyDNSLength(true),
)

// DirectoryErrorCode is a stable, bounded conformance failure class. It never
// contains input data.
type DirectoryErrorCode string

const (
	DirectoryInvalidJSON       DirectoryErrorCode = "invalid_json"
	DirectoryNonCanonicalJSON  DirectoryErrorCode = "noncanonical_json"
	DirectoryUnsupportedSchema DirectoryErrorCode = "unsupported_schema"
	DirectoryBodyTooLarge      DirectoryErrorCode = "body_too_large"
	DirectoryTooManyServers    DirectoryErrorCode = "too_many_servers"
	DirectoryInvalidGeneration DirectoryErrorCode = "invalid_generation"
	DirectoryInvalidFreshness  DirectoryErrorCode = "invalid_freshness"
	DirectoryInvalidIdentity   DirectoryErrorCode = "invalid_identity"
	DirectoryInvalidText       DirectoryErrorCode = "invalid_text"
	DirectoryInvalidRegion     DirectoryErrorCode = "invalid_region"
	DirectoryInvalidProtocol   DirectoryErrorCode = "invalid_protocol"
	DirectoryInvalidContent    DirectoryErrorCode = "invalid_content"
	DirectoryInvalidPlayers    DirectoryErrorCode = "invalid_players"
	DirectoryInvalidStatus     DirectoryErrorCode = "invalid_status"
	DirectoryInvalidEndpoint   DirectoryErrorCode = "invalid_endpoint"
	DirectoryUnorderedServers  DirectoryErrorCode = "unordered_servers"
)

// DirectoryError reports only a stable class so malformed public input cannot
// enter diagnostics. Callers may compare Code or use DirectoryErrorCodeOf.
type DirectoryError struct {
	Code DirectoryErrorCode
}

func (err *DirectoryError) Error() string {
	return "invalid metaserver directory: " + string(err.Code)
}

// DirectoryErrorCodeOf returns a bounded error class and false for unrelated
// errors.
func DirectoryErrorCodeOf(err error) (DirectoryErrorCode, bool) {
	var directoryError *DirectoryError
	if !errors.As(err, &directoryError) {
		return "", false
	}
	return directoryError.Code, true
}

type directoryWire struct {
	Schema      string       `json:"schema"`
	Generation  string       `json:"generation"`
	GeneratedAt string       `json:"generatedAt"`
	ExpiresAt   string       `json:"expiresAt"`
	Servers     []serverWire `json:"servers"`
}

type serverWire struct {
	ServerID          string        `json:"serverId"`
	CertificateSHA256 string        `json:"certificateSha256"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	Region            *string       `json:"region,omitempty"`
	Protocol          protocolWire  `json:"protocol"`
	Content           contentWire   `json:"content"`
	Players           playersWire   `json:"players"`
	Status            string        `json:"status"`
	PasswordRequired  bool          `json:"passwordRequired"`
	Endpoint          *endpointWire `json:"endpoint,omitempty"`
}

type protocolWire struct {
	Major uint32 `json:"major"`
	Minor uint32 `json:"minor"`
}

type contentWire struct {
	ID             string `json:"id"`
	RevisionSHA256 string `json:"revisionSha256"`
}

type playersWire struct {
	Online   uint32 `json:"online"`
	Capacity uint32 `json:"capacity"`
}

type endpointWire struct {
	Hostname string `json:"hostname"`
	Port     uint32 `json:"port"`
}

// ParseDirectoryJSON validates one complete canonical JSON snapshot. Failure
// returns no partial model and does not mutate caller-owned state.
func ParseDirectoryJSON(input []byte) (*metaserverv1.DirectorySnapshot, error) {
	if len(input) > MaximumDirectoryBodyBytes {
		return nil, directoryFailure(DirectoryBodyTooLarge)
	}
	if !utf8.Valid(input) {
		return nil, directoryFailure(DirectoryInvalidJSON)
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var wire directoryWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, directoryFailure(DirectoryNonCanonicalJSON)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, directoryFailure(DirectoryNonCanonicalJSON)
	}

	snapshot, err := directoryFromWire(wire)
	if err != nil {
		return nil, err
	}
	canonical, err := MarshalDirectoryJSON(snapshot)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, input) {
		return nil, directoryFailure(DirectoryNonCanonicalJSON)
	}
	return snapshot, nil
}

// MarshalDirectoryJSON validates and renders a snapshot as canonical bytes.
// The returned allocation is owned by the caller and always ends in one LF.
func MarshalDirectoryJSON(snapshot *metaserverv1.DirectorySnapshot) ([]byte, error) {
	if err := ValidateDirectory(snapshot); err != nil {
		return nil, err
	}
	wire, err := directoryToWire(snapshot)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wire); err != nil {
		return nil, directoryFailure(DirectoryInvalidJSON)
	}
	if encoded.Len() > MaximumDirectoryBodyBytes {
		return nil, directoryFailure(DirectoryBodyTooLarge)
	}
	return encoded.Bytes(), nil
}

// ValidateDirectory enforces semantic bounds without reading a clock or
// retaining references. The snapshot remains caller-owned and mutable.
func ValidateDirectory(snapshot *metaserverv1.DirectorySnapshot) error {
	if snapshot == nil {
		return directoryFailure(DirectoryInvalidJSON)
	}
	if snapshot.Schema != DirectorySchema {
		return directoryFailure(DirectoryUnsupportedSchema)
	}
	if snapshot.Generation == 0 {
		return directoryFailure(DirectoryInvalidGeneration)
	}
	if snapshot.GeneratedAtUnixSeconds > MaximumDirectoryUnixSeconds ||
		snapshot.ExpiresAtUnixSeconds > MaximumDirectoryUnixSeconds ||
		snapshot.ExpiresAtUnixSeconds <= snapshot.GeneratedAtUnixSeconds ||
		snapshot.ExpiresAtUnixSeconds-snapshot.GeneratedAtUnixSeconds > MaximumDirectoryLifetimeSeconds {
		return directoryFailure(DirectoryInvalidFreshness)
	}
	if len(snapshot.Servers) > MaximumDirectoryServers {
		return directoryFailure(DirectoryTooManyServers)
	}

	var previous []byte
	for _, server := range snapshot.Servers {
		if err := validateDirectoryServer(server); err != nil {
			return err
		}
		if previous != nil && bytes.Compare(previous, server.ServerId) >= 0 {
			return directoryFailure(DirectoryUnorderedServers)
		}
		previous = server.ServerId
	}
	return nil
}

// DirectoryFreshAt reports whether a previously validated snapshot is fresh
// at now. It fails closed for nil or structurally invalid snapshots.
func DirectoryFreshAt(snapshot *metaserverv1.DirectorySnapshot, now uint64) bool {
	if ValidateDirectory(snapshot) != nil {
		return false
	}
	if snapshot.GeneratedAtUnixSeconds > now &&
		snapshot.GeneratedAtUnixSeconds-now > MaximumDirectoryFutureSkew {
		return false
	}
	return now < snapshot.ExpiresAtUnixSeconds
}

// DirectoryETag returns the strong representation-specific ETag for canonical
// JSON bytes, including their final LF.
func DirectoryETag(snapshot *metaserverv1.DirectorySnapshot) (string, error) {
	encoded, err := MarshalDirectoryJSON(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return `"` + DirectorySchema + "-sha256-" + hex.EncodeToString(digest[:]) + `"`, nil
}

// DirectoryServerCompatible applies the exact GP1/content filter. Invalid
// server models never match.
func DirectoryServerCompatible(
	server *metaserverv1.DirectoryServer,
	protocolMajor uint32,
	protocolMinor uint32,
	contentID string,
	contentRevisionSHA256 []byte,
) bool {
	return validateDirectoryServer(server) == nil &&
		server.ProtocolMajor == protocolMajor &&
		server.ProtocolMinor == protocolMinor &&
		server.ContentId == contentID &&
		bytes.Equal(server.ContentRevisionSha256, contentRevisionSHA256)
}

func validateDirectoryServer(server *metaserverv1.DirectoryServer) error {
	if server == nil || len(server.ServerId) != sha256.Size ||
		len(server.CertificateSha256) != sha256.Size ||
		!bytes.Equal(server.ServerId, server.CertificateSha256) {
		return directoryFailure(DirectoryInvalidIdentity)
	}
	if !validDirectoryText(server.Name, 1, MaximumDirectoryNameBytes) ||
		!validDirectoryText(server.Description, 0, MaximumDirectoryDescriptionBytes) {
		return directoryFailure(DirectoryInvalidText)
	}
	if server.Region != nil && !validDirectoryIdentifier(*server.Region, MaximumDirectoryRegionBytes, "-") {
		return directoryFailure(DirectoryInvalidRegion)
	}
	if server.ProtocolMajor != 1 || server.ProtocolMinor > 65_535 {
		return directoryFailure(DirectoryInvalidProtocol)
	}
	if !validDirectoryIdentifier(server.ContentId, MaximumDirectoryContentIDBytes, "._-") ||
		len(server.ContentRevisionSha256) != sha256.Size {
		return directoryFailure(DirectoryInvalidContent)
	}
	if server.PlayersCapacity == 0 || server.PlayersCapacity > MaximumDirectoryPlayers ||
		server.PlayersOnline > server.PlayersCapacity {
		return directoryFailure(DirectoryInvalidPlayers)
	}
	switch server.Status {
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_ONLINE:
		if server.PlayersOnline >= server.PlayersCapacity {
			return directoryFailure(DirectoryInvalidStatus)
		}
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL:
		if server.PlayersOnline != server.PlayersCapacity {
			return directoryFailure(DirectoryInvalidStatus)
		}
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_MAINTENANCE:
		if server.PlayersOnline != 0 {
			return directoryFailure(DirectoryInvalidStatus)
		}
	default:
		return directoryFailure(DirectoryInvalidStatus)
	}
	if server.Endpoint != nil && !validDirectoryEndpoint(server.Endpoint) {
		return directoryFailure(DirectoryInvalidEndpoint)
	}
	return nil
}

func directoryFromWire(wire directoryWire) (*metaserverv1.DirectorySnapshot, error) {
	if wire.Schema != DirectorySchema {
		return nil, directoryFailure(DirectoryUnsupportedSchema)
	}
	generation, ok := parseCanonicalUint(wire.Generation)
	if !ok || generation == 0 {
		return nil, directoryFailure(DirectoryInvalidGeneration)
	}
	generatedAt, ok := parseCanonicalUint(wire.GeneratedAt)
	if !ok {
		return nil, directoryFailure(DirectoryInvalidFreshness)
	}
	expiresAt, ok := parseCanonicalUint(wire.ExpiresAt)
	if !ok {
		return nil, directoryFailure(DirectoryInvalidFreshness)
	}
	if len(wire.Servers) > MaximumDirectoryServers {
		return nil, directoryFailure(DirectoryTooManyServers)
	}

	snapshot := &metaserverv1.DirectorySnapshot{
		Schema:                 wire.Schema,
		Generation:             generation,
		GeneratedAtUnixSeconds: generatedAt,
		ExpiresAtUnixSeconds:   expiresAt,
		Servers:                make([]*metaserverv1.DirectoryServer, 0, len(wire.Servers)),
	}
	for _, value := range wire.Servers {
		serverID, err := decodeDirectoryDigest(value.ServerID, DirectoryInvalidIdentity)
		if err != nil {
			return nil, err
		}
		certificate, err := decodeDirectoryDigest(value.CertificateSHA256, DirectoryInvalidIdentity)
		if err != nil {
			return nil, err
		}
		contentRevision, err := decodeDirectoryDigest(value.Content.RevisionSHA256, DirectoryInvalidContent)
		if err != nil {
			return nil, err
		}
		status, ok := directoryStatusFromWire(value.Status)
		if !ok {
			return nil, directoryFailure(DirectoryInvalidStatus)
		}
		server := &metaserverv1.DirectoryServer{
			ServerId:              serverID,
			CertificateSha256:     certificate,
			Name:                  value.Name,
			Description:           value.Description,
			Region:                value.Region,
			ProtocolMajor:         value.Protocol.Major,
			ProtocolMinor:         value.Protocol.Minor,
			ContentId:             value.Content.ID,
			ContentRevisionSha256: contentRevision,
			PlayersOnline:         value.Players.Online,
			PlayersCapacity:       value.Players.Capacity,
			Status:                status,
			PasswordRequired:      value.PasswordRequired,
		}
		if value.Endpoint != nil {
			server.Endpoint = &metaserverv1.DirectEndpoint{
				Hostname: value.Endpoint.Hostname,
				Port:     value.Endpoint.Port,
			}
		}
		snapshot.Servers = append(snapshot.Servers, server)
	}
	if err := ValidateDirectory(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func directoryToWire(snapshot *metaserverv1.DirectorySnapshot) (directoryWire, error) {
	wire := directoryWire{
		Schema:      snapshot.Schema,
		Generation:  strconv.FormatUint(snapshot.Generation, 10),
		GeneratedAt: strconv.FormatUint(snapshot.GeneratedAtUnixSeconds, 10),
		ExpiresAt:   strconv.FormatUint(snapshot.ExpiresAtUnixSeconds, 10),
		Servers:     make([]serverWire, 0, len(snapshot.Servers)),
	}
	for _, server := range snapshot.Servers {
		status, ok := directoryStatusToWire(server.Status)
		if !ok {
			return directoryWire{}, directoryFailure(DirectoryInvalidStatus)
		}
		value := serverWire{
			ServerID:          hex.EncodeToString(server.ServerId),
			CertificateSHA256: hex.EncodeToString(server.CertificateSha256),
			Name:              server.Name,
			Description:       server.Description,
			Region:            server.Region,
			Protocol: protocolWire{
				Major: server.ProtocolMajor,
				Minor: server.ProtocolMinor,
			},
			Content: contentWire{
				ID:             server.ContentId,
				RevisionSHA256: hex.EncodeToString(server.ContentRevisionSha256),
			},
			Players: playersWire{
				Online:   server.PlayersOnline,
				Capacity: server.PlayersCapacity,
			},
			Status:           status,
			PasswordRequired: server.PasswordRequired,
		}
		if server.Endpoint != nil {
			value.Endpoint = &endpointWire{
				Hostname: server.Endpoint.Hostname,
				Port:     server.Endpoint.Port,
			}
		}
		wire.Servers = append(wire.Servers, value)
	}
	return wire, nil
}

func validDirectoryText(value string, minimum int, maximum int) bool {
	if !utf8.ValidString(value) || len(value) < minimum || len(value) > maximum {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || current == '\u2028' || current == '\u2029' {
			return false
		}
	}
	return true
}

func validDirectoryIdentifier(value string, maximum int, interior string) bool {
	if len(value) == 0 || len(value) > maximum || !lowerDirectoryAlphanumeric(value[0]) ||
		!lowerDirectoryAlphanumeric(value[len(value)-1]) {
		return false
	}
	for index := 1; index+1 < len(value); index++ {
		current := value[index]
		if !lowerDirectoryAlphanumeric(current) && !strings.ContainsRune(interior, rune(current)) {
			return false
		}
	}
	return true
}

func validDirectoryEndpoint(endpoint *metaserverv1.DirectEndpoint) bool {
	if endpoint == nil || endpoint.Port == 0 || endpoint.Port > 65_535 ||
		len(endpoint.Hostname) == 0 || len(endpoint.Hostname) > 253 ||
		endpoint.Hostname != strings.ToLower(endpoint.Hostname) ||
		net.ParseIP(endpoint.Hostname) != nil || !strings.Contains(endpoint.Hostname, ".") {
		return false
	}
	hasLetter := false
	hasNonNumericLabel := false
	for _, label := range strings.Split(endpoint.Hostname, ".") {
		if len(label) == 0 || len(label) > 63 || !lowerDirectoryAlphanumeric(label[0]) ||
			!lowerDirectoryAlphanumeric(label[len(label)-1]) {
			return false
		}
		for _, current := range []byte(label) {
			if current >= 'a' && current <= 'z' {
				hasLetter = true
			}
			if !lowerDirectoryAlphanumeric(current) && current != '-' {
				return false
			}
		}
		if strings.HasPrefix(label, "xn--") {
			canonical, err := directoryIDNAProfile.ToASCII(label)
			if err != nil || canonical != label {
				return false
			}
		}
		if !numericDirectoryHostLabel(label) {
			hasNonNumericLabel = true
		}
	}
	return hasLetter && hasNonNumericLabel
}

func numericDirectoryHostLabel(value string) bool {
	digits := value
	base := byte(10)
	if len(value) > 2 && strings.HasPrefix(value, "0x") {
		digits = value[2:]
		base = 16
	}
	for _, current := range []byte(digits) {
		if current >= '0' && current <= '9' {
			continue
		}
		if base == 16 && current >= 'a' && current <= 'f' {
			continue
		}
		return false
	}
	return digits != ""
}

func lowerDirectoryAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func parseCanonicalUint(value string) (uint64, bool) {
	if value == "" || len(value) > 20 || len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	for _, current := range []byte(value) {
		if current < '0' || current > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return parsed, err == nil
}

func decodeDirectoryDigest(value string, code DirectoryErrorCode) ([]byte, error) {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return nil, directoryFailure(code)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, directoryFailure(code)
	}
	return decoded, nil
}

func directoryStatusFromWire(value string) (metaserverv1.DirectoryServerStatus, bool) {
	switch value {
	case "online":
		return metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_ONLINE, true
	case "full":
		return metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL, true
	case "maintenance":
		return metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_MAINTENANCE, true
	default:
		return metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_UNSPECIFIED, false
	}
}

func directoryStatusToWire(value metaserverv1.DirectoryServerStatus) (string, bool) {
	switch value {
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_ONLINE:
		return "online", true
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_FULL:
		return "full", true
	case metaserverv1.DirectoryServerStatus_DIRECTORY_SERVER_STATUS_MAINTENANCE:
		return "maintenance", true
	default:
		return "", false
	}
}

func directoryFailure(code DirectoryErrorCode) error {
	return &DirectoryError{Code: code}
}
