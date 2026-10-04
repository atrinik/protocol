// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

//! Canonical access-route request and resolve-response JSON contracts.

use std::{error, fmt, net::IpAddr, str};

pub const ACCESS_RESOLVE_REQUEST_SCHEMA: &str = "atrinik-access-resolve-v1";
pub const ACCESS_RESOLVED_SCHEMA: &str = "atrinik-access-resolved-v1";
pub const MAXIMUM_ACCESS_RESOLVE_REQUEST_BYTES: usize = 512;
pub const MAXIMUM_ACCESS_RESOLVE_RESPONSE_BYTES: usize = 8_192;
pub const MAXIMUM_ACCESS_CERTIFICATE_DER_BYTES: usize = 2_048;
pub const MAXIMUM_ACCESS_NAME_BYTES: usize = 80;
pub const MAXIMUM_ACCESS_GRANT_LIFETIME_SECONDS: u64 = 15;
pub const ACCESS_UNAVAILABLE_BODY: &[u8] = b"{\"error\":{\"code\":\"access_unavailable\"}}";

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AccessProfile {
    Classic,
    Game,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AccessEndpoint {
    pub hostname: String,
    pub port: u16,
}

#[derive(Clone, Eq, PartialEq)]
pub struct AccessResolved {
    pub profile: AccessProfile,
    pub server_id: [u8; 32],
    pub certificate_der: Vec<u8>,
    pub name: String,
    pub generation: [u8; 32],
    pub client_nonce: [u8; 32],
    pub grant: [u8; 32],
    pub expires_at: u64,
    pub endpoint: Option<AccessEndpoint>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AccessError {
    InvalidJson,
    NonCanonicalJson,
    UnsupportedSchema,
    BodyTooLarge,
    InvalidProfile,
    InvalidIdentity,
    InvalidCertificate,
    InvalidText,
    InvalidPolicy,
    InvalidGeneration,
    InvalidNonce,
    InvalidGrant,
    InvalidFreshness,
    InvalidEndpoint,
}

impl AccessError {
    #[must_use]
    pub const fn code(self) -> &'static str {
        match self {
            Self::InvalidJson => "invalid_json",
            Self::NonCanonicalJson => "noncanonical_json",
            Self::UnsupportedSchema => "unsupported_schema",
            Self::BodyTooLarge => "body_too_large",
            Self::InvalidProfile => "invalid_profile",
            Self::InvalidIdentity => "invalid_identity",
            Self::InvalidCertificate => "invalid_certificate",
            Self::InvalidText => "invalid_text",
            Self::InvalidPolicy => "invalid_policy",
            Self::InvalidGeneration => "invalid_generation",
            Self::InvalidNonce => "invalid_nonce",
            Self::InvalidGrant => "invalid_grant",
            Self::InvalidFreshness => "invalid_freshness",
            Self::InvalidEndpoint => "invalid_endpoint",
        }
    }
}

impl fmt::Display for AccessError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(formatter, "invalid access contract: {}", self.code())
    }
}

impl error::Error for AccessError {}

#[must_use]
pub fn marshal_access_resolve_request(
    route_capability: &[u8; 32],
    client_nonce: &[u8; 32],
) -> Vec<u8> {
    let mut output = Vec::with_capacity(204);
    output.extend_from_slice(b"{\"schema\":\"atrinik-access-resolve-v1\",\"routeCapability\":");
    push_hex_string(&mut output, route_capability);
    output.extend_from_slice(b",\"clientNonce\":");
    push_hex_string(&mut output, client_nonce);
    output.push(b'}');
    output
}

/// Parse bounded syntax and nonce/freshness. The caller MUST verify DER/P-256,
/// serverId = SHA256(DER), and the trusted SPKI pin before sending a credential.
pub fn parse_access_resolved(
    input: &[u8],
    expected_client_nonce: &[u8; 32],
    now: u64,
) -> Result<AccessResolved, AccessError> {
    if input.len() > MAXIMUM_ACCESS_RESOLVE_RESPONSE_BYTES {
        return Err(AccessError::BodyTooLarge);
    }
    if str::from_utf8(input).is_err() {
        return Err(AccessError::InvalidJson);
    }
    let mut parser = Parser::new(input);
    parser.expect(b"{\"schema\":")?;
    let schema = parser.string()?;
    parser.expect(b",\"profile\":")?;
    let profile = match parser.string()?.as_str() {
        "classic" => AccessProfile::Classic,
        "game" => AccessProfile::Game,
        _ => return Err(AccessError::InvalidProfile),
    };
    parser.expect(b",\"serverId\":")?;
    let server_id = parser.hex::<32>(AccessError::InvalidIdentity)?;
    parser.expect(b",\"certificate\":")?;
    let certificate_der = parser.certificate()?;
    parser.expect(b",\"name\":")?;
    let name = parser.string()?;
    parser.expect(b",\"accessRequired\":")?;
    if !parser.consume(b"true") {
        return Err(AccessError::InvalidPolicy);
    }
    parser.expect(b",\"generation\":")?;
    let generation = parser.hex::<32>(AccessError::InvalidGeneration)?;
    parser.expect(b",\"clientNonce\":")?;
    let client_nonce = parser.hex::<32>(AccessError::InvalidNonce)?;
    parser.expect(b",\"grant\":")?;
    let grant = parser.hex::<32>(AccessError::InvalidGrant)?;
    parser.expect(b",\"expiresAt\":")?;
    let expires_at = parser.quoted_u64(AccessError::InvalidFreshness)?;
    let endpoint = if parser.consume(b",\"endpoint\":{\"hostname\":") {
        let hostname = parser.string()?;
        parser.expect(b",\"port\":")?;
        let port = parser.u16()?;
        parser.expect(b"}")?;
        Some(AccessEndpoint { hostname, port })
    } else {
        None
    };
    parser.expect(b"}")?;
    if !parser.finished() {
        return Err(AccessError::NonCanonicalJson);
    }
    if schema != ACCESS_RESOLVED_SCHEMA {
        return Err(AccessError::UnsupportedSchema);
    }
    if !valid_text(&name, 1, MAXIMUM_ACCESS_NAME_BYTES) {
        return Err(AccessError::InvalidText);
    }
    if client_nonce != *expected_client_nonce {
        return Err(AccessError::InvalidNonce);
    }
    if expires_at <= now || expires_at - now > MAXIMUM_ACCESS_GRANT_LIFETIME_SECONDS {
        return Err(AccessError::InvalidFreshness);
    }
    if endpoint
        .as_ref()
        .is_some_and(|value| !valid_endpoint(value))
    {
        return Err(AccessError::InvalidEndpoint);
    }
    Ok(AccessResolved {
        profile,
        server_id,
        certificate_der,
        name,
        generation,
        client_nonce,
        grant,
        expires_at,
        endpoint,
    })
}

#[must_use]
pub fn is_access_unavailable(input: &[u8]) -> bool {
    input == ACCESS_UNAVAILABLE_BODY
}

fn valid_text(value: &str, minimum: usize, maximum: usize) -> bool {
    (minimum..=maximum).contains(&value.len())
        && value.chars().all(|current| {
            !current.is_control()
                && !matches!(current, '\u{2028}' | '\u{2029}' | '\u{fffe}' | '\u{ffff}')
        })
}

fn valid_endpoint(endpoint: &AccessEndpoint) -> bool {
    let hostname = endpoint.hostname.as_str();
    if endpoint.port == 0
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
        if label.starts_with("xn--")
            && idna::domain_to_ascii_strict(label).map_or(true, |canonical| canonical != label)
        {
            return false;
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

fn push_hex_string(output: &mut Vec<u8>, value: &[u8]) {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    output.push(b'"');
    for current in value {
        output.push(HEX[usize::from(current >> 4)]);
        output.push(HEX[usize::from(current & 0x0f)]);
    }
    output.push(b'"');
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

    fn expect(&mut self, expected: &[u8]) -> Result<(), AccessError> {
        if self.consume(expected) {
            Ok(())
        } else {
            Err(AccessError::NonCanonicalJson)
        }
    }

    fn consume(&mut self, expected: &[u8]) -> bool {
        if self.input.get(self.offset..self.offset + expected.len()) == Some(expected) {
            self.offset += expected.len();
            true
        } else {
            false
        }
    }

    fn string(&mut self) -> Result<String, AccessError> {
        self.expect(b"\"")?;
        let mut value = Vec::new();
        loop {
            let current = *self
                .input
                .get(self.offset)
                .ok_or(AccessError::NonCanonicalJson)?;
            self.offset += 1;
            match current {
                b'"' => break,
                b'\\' => {
                    let escaped = *self
                        .input
                        .get(self.offset)
                        .ok_or(AccessError::NonCanonicalJson)?;
                    self.offset += 1;
                    if !matches!(escaped, b'"' | b'\\') {
                        return Err(AccessError::NonCanonicalJson);
                    }
                    value.push(escaped);
                }
                0x00..=0x1f => return Err(AccessError::NonCanonicalJson),
                _ => value.push(current),
            }
        }
        String::from_utf8(value).map_err(|_| AccessError::InvalidJson)
    }

    fn quoted_u64(&mut self, error: AccessError) -> Result<u64, AccessError> {
        let value = self.string()?;
        parse_canonical_u64(&value).ok_or(error)
    }

    fn u16(&mut self) -> Result<u16, AccessError> {
        let start = self.offset;
        while self.input.get(self.offset).is_some_and(u8::is_ascii_digit) {
            self.offset += 1;
        }
        let value = str::from_utf8(&self.input[start..self.offset])
            .map_err(|_| AccessError::InvalidJson)?;
        parse_canonical_u64(value)
            .and_then(|value| u16::try_from(value).ok())
            .filter(|value| *value != 0)
            .ok_or(AccessError::InvalidEndpoint)
    }

    fn hex<const N: usize>(&mut self, error: AccessError) -> Result<[u8; N], AccessError> {
        let value = self.string()?;
        if value.len() != N * 2
            || !value
                .bytes()
                .all(|current| current.is_ascii_digit() || (b'a'..=b'f').contains(&current))
        {
            return Err(error);
        }
        let mut output = [0u8; N];
        for (destination, pair) in output.iter_mut().zip(value.as_bytes().chunks_exact(2)) {
            *destination =
                (hex_value(pair[0]).ok_or(error)? << 4) | hex_value(pair[1]).ok_or(error)?;
        }
        Ok(output)
    }

    fn certificate(&mut self) -> Result<Vec<u8>, AccessError> {
        let value = self.string()?;
        decode_padded_base64(&value).ok_or(AccessError::InvalidCertificate)
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

fn decode_padded_base64(value: &str) -> Option<Vec<u8>> {
    let bytes = value.as_bytes();
    if bytes.is_empty() || !bytes.len().is_multiple_of(4) {
        return None;
    }
    let mut output = Vec::with_capacity(bytes.len() / 4 * 3);
    for (index, group) in bytes.chunks_exact(4).enumerate() {
        let final_group = index + 1 == bytes.len() / 4;
        let first = base64_value(group[0])?;
        let second = base64_value(group[1])?;
        output.push((first << 2) | (second >> 4));
        match (group[2], group[3]) {
            (b'=', b'=') if final_group && second & 0x0f == 0 => {}
            (third, b'=') if final_group => {
                let third = base64_value(third)?;
                if third & 0x03 != 0 {
                    return None;
                }
                output.push((second << 4) | (third >> 2));
            }
            (third, fourth) => {
                let third = base64_value(third)?;
                let fourth = base64_value(fourth)?;
                output.push((second << 4) | (third >> 2));
                output.push((third << 6) | fourth);
            }
        }
        if output.len() > MAXIMUM_ACCESS_CERTIFICATE_DER_BYTES {
            return None;
        }
    }
    (!output.is_empty()).then_some(output)
}

const fn hex_value(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        _ => None,
    }
}

const fn base64_value(value: u8) -> Option<u8> {
    match value {
        b'A'..=b'Z' => Some(value - b'A'),
        b'a'..=b'z' => Some(value - b'a' + 26),
        b'0'..=b'9' => Some(value - b'0' + 52),
        b'+' => Some(62),
        b'/' => Some(63),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CANONICAL: &[u8] = include_bytes!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../fixtures/access-resolve-v1.json"
    ));

    #[test]
    fn request_is_canonical_and_bounded() {
        let request = marshal_access_resolve_request(&[0x11; 32], &[0x22; 32]);
        assert_eq!(request.len(), 204);
        assert!(request.len() <= MAXIMUM_ACCESS_RESOLVE_REQUEST_BYTES);
        assert_eq!(
            str::from_utf8(&request).expect("ASCII"),
            "{\"schema\":\"atrinik-access-resolve-v1\",\"routeCapability\":\"1111111111111111111111111111111111111111111111111111111111111111\",\"clientNonce\":\"2222222222222222222222222222222222222222222222222222222222222222\"}"
        );
    }

    #[test]
    fn canonical_response_parses_without_retaining_route_capability() {
        let resolved = parse_access_resolved(CANONICAL, &[0x22; 32], 1_000).expect("fixture");
        assert_eq!(resolved.profile, AccessProfile::Game);
        assert_eq!(
            resolved.server_id,
            [
                13, 97, 218, 233, 66, 38, 166, 140, 36, 82, 89, 136, 152, 211, 62, 248, 235, 151,
                167, 58, 4, 2, 148, 130, 92, 46, 237, 176, 29, 106, 238, 64
            ]
        );
        assert_eq!(resolved.certificate_der.len(), 318);
        assert_eq!(resolved.generation, [0x11; 32]);
        assert_eq!(resolved.grant, [0x33; 32]);
        assert_eq!(resolved.expires_at, 1_005);
        assert_eq!(
            resolved.endpoint,
            Some(AccessEndpoint {
                hostname: "play.example.org".to_owned(),
                port: 13_327,
            })
        );
    }

    #[test]
    fn nonce_freshness_canonicality_size_and_base64_fail_closed() {
        assert!(matches!(
            parse_access_resolved(CANONICAL, &[0x23; 32], 1_000),
            Err(AccessError::InvalidNonce)
        ));
        assert!(matches!(
            parse_access_resolved(CANONICAL, &[0x22; 32], 1_005),
            Err(AccessError::InvalidFreshness)
        ));
        let mut noncanonical = vec![b' '];
        noncanonical.extend_from_slice(CANONICAL);
        assert!(matches!(
            parse_access_resolved(&noncanonical, &[0x22; 32], 1_000),
            Err(AccessError::NonCanonicalJson)
        ));
        let mut invalid_base64 = String::from_utf8(CANONICAL.to_vec()).expect("fixture");
        let marker = "\"certificate\":\"";
        let start = invalid_base64.find(marker).expect("certificate") + marker.len();
        let end = start + invalid_base64[start..].find('"').expect("end certificate");
        invalid_base64.replace_range(start..end, "MAB=");
        assert!(matches!(
            parse_access_resolved(invalid_base64.as_bytes(), &[0x22; 32], 1_000),
            Err(AccessError::InvalidCertificate)
        ));
        assert!(matches!(
            parse_access_resolved(
                &vec![b' '; MAXIMUM_ACCESS_RESOLVE_RESPONSE_BYTES + 1],
                &[0x22; 32],
                1_000,
            ),
            Err(AccessError::BodyTooLarge)
        ));
    }

    #[test]
    fn unavailable_body_is_one_fixed_shape() {
        assert!(is_access_unavailable(ACCESS_UNAVAILABLE_BODY));
        assert!(!is_access_unavailable(
            b"{\"error\":{\"code\":\"access_expired\"}}"
        ));
    }
}
