// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

use std::{error, fmt, net::IpAddr, str};

use bytes::Bytes;

use super::v1::{DirectEndpoint, DirectoryServer, DirectoryServerStatus, DirectorySnapshot};

pub const DIRECTORY_SCHEMA: &str = "atrinik-directory-v1";
pub const MAXIMUM_DIRECTORY_BODY_BYTES: usize = 262_144;
pub const MAXIMUM_DIRECTORY_SERVERS: usize = 512;
pub const MAXIMUM_DIRECTORY_LIFETIME_SECONDS: u64 = 14_400;
pub const MAXIMUM_DIRECTORY_FUTURE_SKEW: u64 = 300;
pub const MAXIMUM_DIRECTORY_UNIX_SECONDS: u64 = 253_402_300_799;
pub const MAXIMUM_DIRECTORY_NAME_BYTES: usize = 80;
pub const MAXIMUM_DIRECTORY_DESCRIPTION_BYTES: usize = 512;
pub const MAXIMUM_DIRECTORY_REGION_BYTES: usize = 32;
pub const MAXIMUM_DIRECTORY_CONTENT_ID_BYTES: usize = 64;
pub const MAXIMUM_DIRECTORY_PLAYERS: u32 = 100_000;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DirectoryError {
    InvalidJson,
    NonCanonicalJson,
    UnsupportedSchema,
    BodyTooLarge,
    TooManyServers,
    InvalidGeneration,
    InvalidFreshness,
    InvalidIdentity,
    InvalidText,
    InvalidRegion,
    InvalidProtocol,
    InvalidContent,
    InvalidPlayers,
    InvalidStatus,
    InvalidEndpoint,
    UnorderedServers,
}

impl DirectoryError {
    #[must_use]
    pub const fn code(self) -> &'static str {
        match self {
            Self::InvalidJson => "invalid_json",
            Self::NonCanonicalJson => "noncanonical_json",
            Self::UnsupportedSchema => "unsupported_schema",
            Self::BodyTooLarge => "body_too_large",
            Self::TooManyServers => "too_many_servers",
            Self::InvalidGeneration => "invalid_generation",
            Self::InvalidFreshness => "invalid_freshness",
            Self::InvalidIdentity => "invalid_identity",
            Self::InvalidText => "invalid_text",
            Self::InvalidRegion => "invalid_region",
            Self::InvalidProtocol => "invalid_protocol",
            Self::InvalidContent => "invalid_content",
            Self::InvalidPlayers => "invalid_players",
            Self::InvalidStatus => "invalid_status",
            Self::InvalidEndpoint => "invalid_endpoint",
            Self::UnorderedServers => "unordered_servers",
        }
    }
}

impl fmt::Display for DirectoryError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(formatter, "invalid metaserver directory: {}", self.code())
    }
}

impl error::Error for DirectoryError {}

/// Parses one complete canonical JSON snapshot. Failure returns no partial
/// model and does not mutate caller-owned state.
pub fn parse_directory_json(input: &[u8]) -> Result<DirectorySnapshot, DirectoryError> {
    if input.len() > MAXIMUM_DIRECTORY_BODY_BYTES {
        return Err(DirectoryError::BodyTooLarge);
    }
    if str::from_utf8(input).is_err() {
        return Err(DirectoryError::InvalidJson);
    }

    let mut parser = Parser::new(input);
    parser.expect(b"{\"schema\":")?;
    let schema = parser.string()?;
    parser.expect(b",\"generation\":")?;
    let generation = parser.quoted_u64(DirectoryError::InvalidGeneration)?;
    parser.expect(b",\"generatedAt\":")?;
    let generated_at_unix_seconds = parser.quoted_u64(DirectoryError::InvalidFreshness)?;
    parser.expect(b",\"expiresAt\":")?;
    let expires_at_unix_seconds = parser.quoted_u64(DirectoryError::InvalidFreshness)?;
    parser.expect(b",\"servers\":[")?;

    let mut servers = Vec::new();
    if !parser.peek(b']') {
        loop {
            if servers.len() == MAXIMUM_DIRECTORY_SERVERS {
                return Err(DirectoryError::TooManyServers);
            }
            servers.push(parser.server()?);
            if parser.consume(b",") {
                continue;
            }
            break;
        }
    }
    parser.expect(b"]}\n")?;
    if !parser.finished() {
        return Err(DirectoryError::NonCanonicalJson);
    }

    let snapshot = DirectorySnapshot {
        schema,
        generation,
        generated_at_unix_seconds,
        expires_at_unix_seconds,
        servers,
    };
    validate_directory(&snapshot)?;
    Ok(snapshot)
}

/// Validates and renders one snapshot as canonical bytes ending in one LF.
pub fn marshal_directory_json(snapshot: &DirectorySnapshot) -> Result<Vec<u8>, DirectoryError> {
    validate_directory(snapshot)?;

    let mut output = Vec::new();
    output.extend_from_slice(b"{\"schema\":");
    push_string(&mut output, &snapshot.schema);
    output.extend_from_slice(b",\"generation\":");
    push_string(&mut output, &snapshot.generation.to_string());
    output.extend_from_slice(b",\"generatedAt\":");
    push_string(&mut output, &snapshot.generated_at_unix_seconds.to_string());
    output.extend_from_slice(b",\"expiresAt\":");
    push_string(&mut output, &snapshot.expires_at_unix_seconds.to_string());
    output.extend_from_slice(b",\"servers\":[");
    for (index, server) in snapshot.servers.iter().enumerate() {
        if index != 0 {
            output.push(b',');
        }
        push_server(&mut output, server)?;
    }
    output.extend_from_slice(b"]}\n");
    if output.len() > MAXIMUM_DIRECTORY_BODY_BYTES {
        return Err(DirectoryError::BodyTooLarge);
    }
    Ok(output)
}

/// Enforces semantic bounds without consulting a clock or retaining any
/// references into the caller-owned snapshot.
pub fn validate_directory(snapshot: &DirectorySnapshot) -> Result<(), DirectoryError> {
    if snapshot.schema != DIRECTORY_SCHEMA {
        return Err(DirectoryError::UnsupportedSchema);
    }
    if snapshot.generation == 0 {
        return Err(DirectoryError::InvalidGeneration);
    }
    if snapshot.generated_at_unix_seconds > MAXIMUM_DIRECTORY_UNIX_SECONDS
        || snapshot.expires_at_unix_seconds > MAXIMUM_DIRECTORY_UNIX_SECONDS
        || snapshot.expires_at_unix_seconds <= snapshot.generated_at_unix_seconds
        || snapshot.expires_at_unix_seconds - snapshot.generated_at_unix_seconds
            > MAXIMUM_DIRECTORY_LIFETIME_SECONDS
    {
        return Err(DirectoryError::InvalidFreshness);
    }
    if snapshot.servers.len() > MAXIMUM_DIRECTORY_SERVERS {
        return Err(DirectoryError::TooManyServers);
    }

    let mut previous: Option<&[u8]> = None;
    for server in &snapshot.servers {
        validate_server(server)?;
        if previous.is_some_and(|value| value >= server.server_id.as_ref()) {
            return Err(DirectoryError::UnorderedServers);
        }
        previous = Some(server.server_id.as_ref());
    }
    Ok(())
}

/// Reports freshness at `now`. Nil is not representable in Rust; malformed
/// snapshots still fail closed.
#[must_use]
pub fn directory_fresh_at(snapshot: &DirectorySnapshot, now: u64) -> bool {
    if validate_directory(snapshot).is_err() {
        return false;
    }
    if snapshot.generated_at_unix_seconds > now
        && snapshot.generated_at_unix_seconds - now > MAXIMUM_DIRECTORY_FUTURE_SKEW
    {
        return false;
    }
    now < snapshot.expires_at_unix_seconds
}

/// Applies exact GP1/content compatibility matching. Invalid server models
/// never match.
#[must_use]
pub fn directory_server_compatible(
    server: &DirectoryServer,
    protocol_major: u32,
    protocol_minor: u32,
    content_id: &str,
    content_revision_sha256: &[u8],
) -> bool {
    validate_server(server).is_ok()
        && server.protocol_major == protocol_major
        && server.protocol_minor == protocol_minor
        && server.content_id == content_id
        && server.content_revision_sha256.as_ref() == content_revision_sha256
}

fn validate_server(server: &DirectoryServer) -> Result<(), DirectoryError> {
    if server.server_id.len() != 32
        || server.certificate_sha256.len() != 32
        || server.server_id != server.certificate_sha256
    {
        return Err(DirectoryError::InvalidIdentity);
    }
    if !valid_text(&server.name, 1, MAXIMUM_DIRECTORY_NAME_BYTES)
        || !valid_text(&server.description, 0, MAXIMUM_DIRECTORY_DESCRIPTION_BYTES)
    {
        return Err(DirectoryError::InvalidText);
    }
    if server
        .region
        .as_ref()
        .is_some_and(|value| !valid_identifier(value, MAXIMUM_DIRECTORY_REGION_BYTES, b"-"))
    {
        return Err(DirectoryError::InvalidRegion);
    }
    if server.protocol_major != 1 || server.protocol_minor > 65_535 {
        return Err(DirectoryError::InvalidProtocol);
    }
    if !valid_identifier(
        &server.content_id,
        MAXIMUM_DIRECTORY_CONTENT_ID_BYTES,
        b"._-",
    ) || server.content_revision_sha256.len() != 32
    {
        return Err(DirectoryError::InvalidContent);
    }
    if server.players_capacity == 0
        || server.players_capacity > MAXIMUM_DIRECTORY_PLAYERS
        || server.players_online > server.players_capacity
    {
        return Err(DirectoryError::InvalidPlayers);
    }
    match DirectoryServerStatus::try_from(server.status) {
        Ok(DirectoryServerStatus::Online) if server.players_online < server.players_capacity => {}
        Ok(DirectoryServerStatus::Full) if server.players_online == server.players_capacity => {}
        Ok(DirectoryServerStatus::Maintenance) if server.players_online == 0 => {}
        _ => return Err(DirectoryError::InvalidStatus),
    }
    if server
        .endpoint
        .as_ref()
        .is_some_and(|endpoint| !valid_endpoint(endpoint))
    {
        return Err(DirectoryError::InvalidEndpoint);
    }
    Ok(())
}

fn valid_text(value: &str, minimum: usize, maximum: usize) -> bool {
    (minimum..=maximum).contains(&value.len())
        && value
            .chars()
            .all(|current| !current.is_control() && !matches!(current, '\u{2028}' | '\u{2029}'))
}

fn valid_identifier(value: &str, maximum: usize, interior: &[u8]) -> bool {
    let bytes = value.as_bytes();
    let Some(first) = bytes.first() else {
        return false;
    };
    let Some(last) = bytes.last() else {
        return false;
    };
    bytes.len() <= maximum
        && lower_alphanumeric(*first)
        && lower_alphanumeric(*last)
        && bytes
            .iter()
            .skip(1)
            .take(bytes.len().saturating_sub(2))
            .all(|current| lower_alphanumeric(*current) || interior.contains(current))
}

fn valid_endpoint(endpoint: &DirectEndpoint) -> bool {
    let hostname = endpoint.hostname.as_str();
    if endpoint.port == 0
        || endpoint.port > 65_535
        || hostname.is_empty()
        || hostname.len() > 253
        || !hostname.contains('.')
        || hostname.parse::<IpAddr>().is_ok()
    {
        return false;
    }
    let mut has_letter = false;
    let mut has_non_numeric_label = false;
    for label in hostname.split('.') {
        let bytes = label.as_bytes();
        if bytes.is_empty()
            || bytes.len() > 63
            || !lower_alphanumeric(bytes[0])
            || !lower_alphanumeric(bytes[bytes.len() - 1])
        {
            return false;
        }
        for current in bytes {
            has_letter |= current.is_ascii_lowercase();
            if !lower_alphanumeric(*current) && *current != b'-' {
                return false;
            }
        }
        has_non_numeric_label |= !numeric_host_label(label);
    }
    has_letter && has_non_numeric_label
}

fn numeric_host_label(value: &str) -> bool {
    let digits = value.strip_prefix("0x").unwrap_or(value);
    !digits.is_empty()
        && if value.starts_with("0x") {
            digits.bytes().all(|current| current.is_ascii_hexdigit())
        } else {
            digits.bytes().all(|current| current.is_ascii_digit())
        }
}

const fn lower_alphanumeric(value: u8) -> bool {
    value.is_ascii_lowercase() || value.is_ascii_digit()
}

fn push_server(output: &mut Vec<u8>, server: &DirectoryServer) -> Result<(), DirectoryError> {
    output.extend_from_slice(b"{\"serverId\":");
    push_hex_string(output, &server.server_id);
    output.extend_from_slice(b",\"certificateSha256\":");
    push_hex_string(output, &server.certificate_sha256);
    output.extend_from_slice(b",\"name\":");
    push_string(output, &server.name);
    output.extend_from_slice(b",\"description\":");
    push_string(output, &server.description);
    if let Some(region) = &server.region {
        output.extend_from_slice(b",\"region\":");
        push_string(output, region);
    }
    output.extend_from_slice(b",\"protocol\":{\"major\":");
    push_u32(output, server.protocol_major);
    output.extend_from_slice(b",\"minor\":");
    push_u32(output, server.protocol_minor);
    output.extend_from_slice(b"},\"content\":{\"id\":");
    push_string(output, &server.content_id);
    output.extend_from_slice(b",\"revisionSha256\":");
    push_hex_string(output, &server.content_revision_sha256);
    output.extend_from_slice(b"},\"players\":{\"online\":");
    push_u32(output, server.players_online);
    output.extend_from_slice(b",\"capacity\":");
    push_u32(output, server.players_capacity);
    output.extend_from_slice(b"},\"status\":");
    let status = match DirectoryServerStatus::try_from(server.status) {
        Ok(DirectoryServerStatus::Online) => "online",
        Ok(DirectoryServerStatus::Full) => "full",
        Ok(DirectoryServerStatus::Maintenance) => "maintenance",
        _ => return Err(DirectoryError::InvalidStatus),
    };
    push_string(output, status);
    output.extend_from_slice(b",\"passwordRequired\":");
    output.extend_from_slice(if server.password_required {
        b"true"
    } else {
        b"false"
    });
    if let Some(endpoint) = &server.endpoint {
        output.extend_from_slice(b",\"endpoint\":{\"hostname\":");
        push_string(output, &endpoint.hostname);
        output.extend_from_slice(b",\"port\":");
        push_u32(output, endpoint.port);
        output.push(b'}');
    }
    output.push(b'}');
    Ok(())
}

fn push_string(output: &mut Vec<u8>, value: &str) {
    output.push(b'"');
    for current in value.as_bytes() {
        if matches!(current, b'"' | b'\\') {
            output.push(b'\\');
        }
        output.push(*current);
    }
    output.push(b'"');
}

fn push_hex_string(output: &mut Vec<u8>, value: &[u8]) {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    output.push(b'"');
    for current in value {
        output.push(HEX[usize::from(current >> 4)]);
        output.push(HEX[usize::from(current & 0x0f)]);
    }
    output.push(b'"');
}

fn push_u32(output: &mut Vec<u8>, value: u32) {
    output.extend_from_slice(value.to_string().as_bytes());
}

struct Parser<'a> {
    input: &'a [u8],
    offset: usize,
}

impl<'a> Parser<'a> {
    const fn new(input: &'a [u8]) -> Self {
        Self { input, offset: 0 }
    }

    fn finished(&self) -> bool {
        self.offset == self.input.len()
    }

    fn peek(&self, value: u8) -> bool {
        self.input.get(self.offset) == Some(&value)
    }

    fn expect(&mut self, expected: &[u8]) -> Result<(), DirectoryError> {
        if !self.consume(expected) {
            return Err(DirectoryError::NonCanonicalJson);
        }
        Ok(())
    }

    fn consume(&mut self, expected: &[u8]) -> bool {
        if self.input.get(self.offset..self.offset + expected.len()) == Some(expected) {
            self.offset += expected.len();
            true
        } else {
            false
        }
    }

    fn string(&mut self) -> Result<String, DirectoryError> {
        self.expect(b"\"")?;
        let mut value = Vec::new();
        loop {
            let current = *self
                .input
                .get(self.offset)
                .ok_or(DirectoryError::NonCanonicalJson)?;
            self.offset += 1;
            match current {
                b'"' => break,
                b'\\' => {
                    let escaped = *self
                        .input
                        .get(self.offset)
                        .ok_or(DirectoryError::NonCanonicalJson)?;
                    self.offset += 1;
                    if !matches!(escaped, b'"' | b'\\') {
                        return Err(DirectoryError::NonCanonicalJson);
                    }
                    value.push(escaped);
                }
                0x00..=0x1f => return Err(DirectoryError::NonCanonicalJson),
                _ => value.push(current),
            }
        }
        String::from_utf8(value).map_err(|_| DirectoryError::InvalidJson)
    }

    fn quoted_u64(&mut self, error: DirectoryError) -> Result<u64, DirectoryError> {
        let value = self.string()?;
        parse_canonical_u64(&value).ok_or(error)
    }

    fn u32(&mut self) -> Result<u32, DirectoryError> {
        let start = self.offset;
        while self.input.get(self.offset).is_some_and(u8::is_ascii_digit) {
            self.offset += 1;
        }
        let value = str::from_utf8(&self.input[start..self.offset])
            .map_err(|_| DirectoryError::InvalidJson)?;
        let parsed = parse_canonical_u64(value).ok_or(DirectoryError::NonCanonicalJson)?;
        u32::try_from(parsed).map_err(|_| DirectoryError::NonCanonicalJson)
    }

    fn boolean(&mut self) -> Result<bool, DirectoryError> {
        if self.consume(b"true") {
            return Ok(true);
        }
        if self.consume(b"false") {
            return Ok(false);
        }
        Err(DirectoryError::NonCanonicalJson)
    }

    fn server(&mut self) -> Result<DirectoryServer, DirectoryError> {
        self.expect(b"{\"serverId\":")?;
        let server_id = self.digest(DirectoryError::InvalidIdentity)?;
        self.expect(b",\"certificateSha256\":")?;
        let certificate_sha256 = self.digest(DirectoryError::InvalidIdentity)?;
        self.expect(b",\"name\":")?;
        let name = self.string()?;
        self.expect(b",\"description\":")?;
        let description = self.string()?;
        let region = if self.consume(b",\"region\":") {
            Some(self.string()?)
        } else {
            None
        };
        self.expect(b",\"protocol\":{\"major\":")?;
        let protocol_major = self.u32()?;
        self.expect(b",\"minor\":")?;
        let protocol_minor = self.u32()?;
        self.expect(b"},\"content\":{\"id\":")?;
        let content_id = self.string()?;
        self.expect(b",\"revisionSha256\":")?;
        let content_revision_sha256 = self.digest(DirectoryError::InvalidContent)?;
        self.expect(b"},\"players\":{\"online\":")?;
        let players_online = self.u32()?;
        self.expect(b",\"capacity\":")?;
        let players_capacity = self.u32()?;
        self.expect(b"},\"status\":")?;
        let status = match self.string()?.as_str() {
            "online" => DirectoryServerStatus::Online,
            "full" => DirectoryServerStatus::Full,
            "maintenance" => DirectoryServerStatus::Maintenance,
            _ => return Err(DirectoryError::InvalidStatus),
        };
        self.expect(b",\"passwordRequired\":")?;
        let password_required = self.boolean()?;
        let endpoint = if self.consume(b",\"endpoint\":{\"hostname\":") {
            let hostname = self.string()?;
            self.expect(b",\"port\":")?;
            let port = self.u32()?;
            self.expect(b"}")?;
            Some(DirectEndpoint { hostname, port })
        } else {
            None
        };
        self.expect(b"}")?;
        Ok(DirectoryServer {
            server_id,
            certificate_sha256,
            name,
            description,
            region,
            protocol_major,
            protocol_minor,
            content_id,
            content_revision_sha256,
            players_online,
            players_capacity,
            status: status as i32,
            password_required,
            endpoint,
        })
    }

    fn digest(&mut self, error: DirectoryError) -> Result<Bytes, DirectoryError> {
        let value = self.string()?;
        if value.len() != 64
            || !value
                .bytes()
                .all(|current| current.is_ascii_hexdigit() && !current.is_ascii_uppercase())
        {
            return Err(error);
        }
        let mut decoded = Vec::with_capacity(32);
        for pair in value.as_bytes().chunks_exact(2) {
            decoded
                .push((hex_value(pair[0]).ok_or(error)? << 4) | hex_value(pair[1]).ok_or(error)?);
        }
        Ok(Bytes::from(decoded))
    }
}

fn parse_canonical_u64(value: &str) -> Option<u64> {
    if value.is_empty()
        || value.len() > 20
        || value.len() > 1 && value.starts_with('0')
        || !value.bytes().all(|current| current.is_ascii_digit())
    {
        return None;
    }
    value.parse().ok()
}

const fn hex_value(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use bytes::Bytes;

    use super::{
        DIRECTORY_SCHEMA, DirectoryError, MAXIMUM_DIRECTORY_BODY_BYTES,
        MAXIMUM_DIRECTORY_CONTENT_ID_BYTES, MAXIMUM_DIRECTORY_DESCRIPTION_BYTES,
        MAXIMUM_DIRECTORY_LIFETIME_SECONDS, MAXIMUM_DIRECTORY_NAME_BYTES,
        MAXIMUM_DIRECTORY_PLAYERS, MAXIMUM_DIRECTORY_REGION_BYTES, MAXIMUM_DIRECTORY_SERVERS,
        MAXIMUM_DIRECTORY_UNIX_SECONDS, directory_fresh_at, directory_server_compatible,
        marshal_directory_json, parse_directory_json, validate_directory,
    };
    use crate::metaserver::v1::{
        DirectEndpoint, DirectoryServer, DirectoryServerStatus, DirectorySnapshot,
    };

    const CANONICAL: &[u8] = include_bytes!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../fixtures/metaserver-directory-v1/canonical.json"
    ));

    const NEGATIVE: &[(&[u8], DirectoryError)] = &[
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-unsupported-schema.json"
            )),
            DirectoryError::UnsupportedSchema,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-zero-generation.json"
            )),
            DirectoryError::InvalidGeneration,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-expired-at-generation.json"
            )),
            DirectoryError::InvalidFreshness,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-identity-mismatch.json"
            )),
            DirectoryError::InvalidIdentity,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-numeric-endpoint.json"
            )),
            DirectoryError::InvalidEndpoint,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-status-count.json"
            )),
            DirectoryError::InvalidStatus,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-unordered-servers.json"
            )),
            DirectoryError::UnorderedServers,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-duplicate-server.json"
            )),
            DirectoryError::UnorderedServers,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-noncanonical-whitespace.json"
            )),
            DirectoryError::NonCanonicalJson,
        ),
        (
            include_bytes!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../../fixtures/metaserver-directory-v1/negative-private-field.json"
            )),
            DirectoryError::NonCanonicalJson,
        ),
    ];

    #[test]
    fn consumes_language_neutral_fixtures() {
        let snapshot = parse_directory_json(CANONICAL).expect("canonical fixture");
        assert_eq!(snapshot.schema, DIRECTORY_SCHEMA);
        assert_eq!(snapshot.servers.len(), 2);
        assert!(snapshot.servers[0].region.is_some());
        assert!(snapshot.servers[0].endpoint.is_some());
        assert!(snapshot.servers[1].region.is_none());
        assert!(snapshot.servers[1].endpoint.is_none());
        assert_eq!(
            marshal_directory_json(&snapshot).expect("render fixture"),
            CANONICAL
        );

        for (input, expected) in NEGATIVE {
            assert_eq!(parse_directory_json(input), Err(*expected));
            assert!(!expected.code().is_empty());
        }
    }

    #[test]
    fn accepts_maximum_bound_models() {
        let maximum_hostname = [
            "a".repeat(63),
            "b".repeat(63),
            "c".repeat(63),
            "d".repeat(61),
        ]
        .join(".");
        assert_eq!(maximum_hostname.len(), 253);

        let mut maximum = valid_snapshot();
        maximum.generated_at_unix_seconds =
            MAXIMUM_DIRECTORY_UNIX_SECONDS - MAXIMUM_DIRECTORY_LIFETIME_SECONDS;
        maximum.expires_at_unix_seconds = MAXIMUM_DIRECTORY_UNIX_SECONDS;
        maximum.servers[0].name = "n".repeat(MAXIMUM_DIRECTORY_NAME_BYTES);
        maximum.servers[0].description = "d".repeat(MAXIMUM_DIRECTORY_DESCRIPTION_BYTES);
        maximum.servers[0].region = Some("r".repeat(MAXIMUM_DIRECTORY_REGION_BYTES));
        maximum.servers[0].content_id = "c".repeat(MAXIMUM_DIRECTORY_CONTENT_ID_BYTES);
        maximum.servers[0].players_online = MAXIMUM_DIRECTORY_PLAYERS;
        maximum.servers[0].players_capacity = MAXIMUM_DIRECTORY_PLAYERS;
        maximum.servers[0].status = DirectoryServerStatus::Full as i32;
        maximum.servers[0].endpoint = Some(DirectEndpoint {
            hostname: maximum_hostname,
            port: 65_535,
        });
        let rendered = marshal_directory_json(&maximum).expect("maximum fields");
        assert_eq!(parse_directory_json(&rendered), Ok(maximum));

        let mut maximum_count = valid_snapshot();
        maximum_count.servers.clear();
        for index in 1..=MAXIMUM_DIRECTORY_SERVERS {
            let mut server = valid_server();
            let mut server_id = [0_u8; 32];
            server_id[28..].copy_from_slice(&(index as u32).to_be_bytes());
            server.server_id = Bytes::copy_from_slice(&server_id);
            server.certificate_sha256 = server.server_id.clone();
            maximum_count.servers.push(server);
        }
        let rendered = marshal_directory_json(&maximum_count).expect("maximum server count");
        assert!(rendered.len() <= MAXIMUM_DIRECTORY_BODY_BYTES);
        assert_eq!(parse_directory_json(&rendered), Ok(maximum_count));

        let mut too_many = valid_snapshot();
        too_many.servers = vec![valid_server(); MAXIMUM_DIRECTORY_SERVERS + 1];
        assert_eq!(
            validate_directory(&too_many),
            Err(DirectoryError::TooManyServers)
        );
        assert_eq!(
            parse_directory_json(&vec![b'x'; MAXIMUM_DIRECTORY_BODY_BYTES + 1]),
            Err(DirectoryError::BodyTooLarge)
        );
    }

    #[test]
    fn filters_exact_compatibility_and_freshness() {
        let snapshot = valid_snapshot();
        let server = &snapshot.servers[0];
        assert!(directory_fresh_at(
            &snapshot,
            snapshot.generated_at_unix_seconds
        ));
        assert!(!directory_fresh_at(
            &snapshot,
            snapshot.expires_at_unix_seconds
        ));

        let mut future = valid_snapshot();
        future.generated_at_unix_seconds = 1_000;
        future.expires_at_unix_seconds = 2_000;
        assert!(directory_fresh_at(&future, 700));
        assert!(!directory_fresh_at(&future, 699));

        assert!(directory_server_compatible(
            server,
            1,
            0,
            "atrinik-main",
            &server.content_revision_sha256,
        ));
        assert!(!directory_server_compatible(
            server,
            1,
            1,
            "atrinik-main",
            &server.content_revision_sha256,
        ));
        assert!(!directory_server_compatible(
            server,
            1,
            0,
            "other",
            &server.content_revision_sha256,
        ));
    }

    #[test]
    fn rejects_unsafe_endpoints_and_model_states() {
        let mut one_byte_identifiers = valid_snapshot();
        one_byte_identifiers.servers[0].region = Some("x".into());
        one_byte_identifiers.servers[0].content_id = "a".into();
        assert_eq!(validate_directory(&one_byte_identifiers), Ok(()));

        for hostname in [
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
        ] {
            let mut value = valid_snapshot();
            value.servers[0]
                .endpoint
                .as_mut()
                .expect("test endpoint")
                .hostname = hostname.into();
            assert_eq!(
                validate_directory(&value),
                Err(DirectoryError::InvalidEndpoint)
            );
        }

        let mut split_identity = valid_snapshot();
        let mut wrong_certificate = split_identity.servers[0].certificate_sha256.to_vec();
        wrong_certificate[0] ^= 1;
        split_identity.servers[0].certificate_sha256 = Bytes::from(wrong_certificate);
        assert_eq!(
            validate_directory(&split_identity),
            Err(DirectoryError::InvalidIdentity)
        );
        let mut unsafe_text = valid_snapshot();
        unsafe_text.servers[0].name = "unsafe\nname".into();
        assert_eq!(
            validate_directory(&unsafe_text),
            Err(DirectoryError::InvalidText)
        );
        let mut unsafe_content = valid_snapshot();
        unsafe_content.servers[0].content_id = "../private".into();
        assert_eq!(
            validate_directory(&unsafe_content),
            Err(DirectoryError::InvalidContent)
        );
    }

    #[test]
    fn parser_is_panic_free_for_truncation_and_byte_mutation() {
        for length in 0..CANONICAL.len() {
            assert!(parse_directory_json(&CANONICAL[..length]).is_err());
        }
        for offset in 0..CANONICAL.len() {
            let mut mutated = CANONICAL.to_vec();
            mutated[offset] ^= 0x80;
            if let Ok(snapshot) = parse_directory_json(&mutated) {
                assert_eq!(
                    marshal_directory_json(&snapshot).expect("mutated canonical model"),
                    mutated
                );
            }
        }
        assert_eq!(
            parse_directory_json(b"\xff"),
            Err(DirectoryError::InvalidJson)
        );
    }

    fn valid_snapshot() -> DirectorySnapshot {
        DirectorySnapshot {
            schema: DIRECTORY_SCHEMA.into(),
            generation: 1,
            generated_at_unix_seconds: 1,
            expires_at_unix_seconds: 2,
            servers: vec![valid_server()],
        }
    }

    fn valid_server() -> DirectoryServer {
        DirectoryServer {
            server_id: Bytes::from(vec![1; 32]),
            certificate_sha256: Bytes::from(vec![1; 32]),
            name: "Server".into(),
            description: String::new(),
            region: None,
            protocol_major: 1,
            protocol_minor: 0,
            content_id: "atrinik-main".into(),
            content_revision_sha256: Bytes::from(vec![2; 32]),
            players_online: 0,
            players_capacity: 1,
            status: DirectoryServerStatus::Online as i32,
            password_required: false,
            endpoint: Some(DirectEndpoint {
                hostname: "server.example.org".into(),
                port: 13_327,
            }),
        }
    }
}
