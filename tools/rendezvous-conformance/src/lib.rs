// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

#[cfg(test)]
mod tests {
    use hmac::{Hmac, Mac};
    use serde_json::Value;
    use sha2::{Digest, Sha256};

    const MAX_SECONDS: u64 = 253402300799;
    const SCHEMA: &str = "atrinik-rendezvous-admission-v1";
    fn lower_hex(s: &str, width: usize) -> bool {
        s.len() == width
            && s.bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    }
    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }
    fn binary(s: &str) -> Vec<u8> {
        s.as_bytes()
            .chunks_exact(2)
            .map(|p| u8::from_str_radix(std::str::from_utf8(p).unwrap(), 16).unwrap())
            .collect()
    }
    fn text<'a>(value: &'a Value, key: &str) -> &'a str {
        value[key].as_str().unwrap()
    }
    fn token_fields(token: &str) -> Option<(Vec<&str>, u64)> {
        let fields: Vec<_> = token.split('.').collect();
        if fields.len() != 6
            || fields[0] != "v1"
            || fields[1].is_empty()
            || fields[1].len() > 32
            || !fields[1]
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
            || fields[2].is_empty()
            || fields[2].starts_with('0')
            || fields[2].len() > 12
            || !fields[2].bytes().all(|b| b.is_ascii_digit())
            || !lower_hex(fields[3], 64)
            || !lower_hex(fields[4], 32)
            || !lower_hex(fields[5], 64)
        {
            return None;
        }
        let issued: u64 = fields[2].parse().ok()?;
        if issued > MAX_SECONDS - 60 {
            return None;
        }
        Some((fields, issued))
    }
    fn signing(authority: &str, server: &str, prefix: &str) -> Vec<u8> {
        format!("atrinik-rendezvous-admission-v1\0{authority}\0classic\0{server}\0{prefix}")
            .into_bytes()
    }
    fn work(token: &str, solution: &str) -> Vec<u8> {
        format!("atrinik-rendezvous-work-v1\0{token}\0{solution}").into_bytes()
    }
    fn mac(key: &[u8], input: &[u8]) -> Hmac<Sha256> {
        let mut h = Hmac::<Sha256>::new_from_slice(key).unwrap();
        h.update(input);
        h
    }
    fn verify(v: &Value, now: u64, role: &str, key_id: &str, key: &[u8]) -> bool {
        let token = text(v, "token");
        let solution = text(v, "solution");
        let ticket = text(v, "ticket");
        if token.len() + 1 + solution.len() > 256
            || !lower_hex(solution, 16)
            || !lower_hex(ticket, 64)
            || !lower_hex(text(v, "serverId"), 64)
        {
            return false;
        }
        let Some((fields, issued)) = token_fields(token) else {
            return false;
        };
        if now < issued
            || now >= issued + 60
            || fields[3] != hex(&Sha256::digest(binary(ticket)))
            || fields[1] != key_id
            || role == "retired"
        {
            return false;
        }
        let prefix = token.rsplit_once('.').unwrap().0;
        if mac(
            key,
            &signing(text(v, "authority"), text(v, "serverId"), prefix),
        )
        .verify_slice(&binary(fields[5]))
        .is_err()
        {
            return false;
        }
        Sha256::digest(work(token, solution))[..3] == [0, 0, 0]
    }
    fn request(raw: &str) -> bool {
        if raw.len() > 256 {
            return false;
        }
        let Ok(v) = serde_json::from_str::<Value>(raw) else {
            return false;
        };
        let Some(hash) = v["ticketHash"].as_str() else {
            return false;
        };
        lower_hex(hash, 64) && raw == format!(r#"{{"schema":"{SCHEMA}","ticketHash":"{hash}"}}"#)
    }
    fn response(raw: &str) -> bool {
        if raw.len() > 512 {
            return false;
        }
        let Ok(v) = serde_json::from_str::<Value>(raw) else {
            return false;
        };
        let (Some(token), Some(expiry)) = (v["challenge"].as_str(), v["expiresAt"].as_str()) else {
            return false;
        };
        let Some((_, issued)) = token_fields(token) else {
            return false;
        };
        expiry == (issued + 60).to_string()
            && raw
                == format!(
                    r#"{{"schema":"{SCHEMA}","challenge":"{token}","difficulty":24,"expiresAt":"{expiry}"}}"#
                )
    }
    #[test]
    fn independent_shared_admission_fixtures() {
        let f: Value = serde_json::from_str(include_str!(
            "../../../fixtures/rendezvous-admission-v1.json"
        ))
        .unwrap();
        assert_eq!(f["synthetic"], true);
        let key = binary(text(&f, "keyHex"));
        assert_eq!(key.len(), 32);
        assert_eq!(
            hex(&Sha256::digest(binary(text(&f, "ticket")))),
            text(&f, "ticketHash")
        );
        assert_ne!(
            hex(&Sha256::digest(text(&f, "ticket").as_bytes())),
            text(&f, "ticketHash")
        );
        let input = signing(
            text(&f, "authority"),
            text(&f, "serverId"),
            text(&f, "tokenPrefix"),
        );
        assert_eq!(hex(&input), text(&f, "hmacInputHex"));
        assert_eq!(
            hex(&mac(&key, &input).finalize().into_bytes()),
            text(&f, "mac")
        );
        assert_eq!(
            format!("{}.{}", text(&f, "tokenPrefix"), text(&f, "mac")),
            text(&f, "token")
        );
        let input = work(text(&f, "token"), text(&f, "solution"));
        assert_eq!(hex(&input), text(&f, "workInputHex"));
        assert_eq!(hex(&Sha256::digest(input)), text(&f, "workDigest"));
        for c in f["cases"].as_array().unwrap() {
            let mut v = f.clone();
            if let Some(overrides) = c["override"].as_object() {
                for (k, value) in overrides {
                    v[k] = value.clone();
                }
            }
            assert_eq!(
                verify(
                    &v,
                    c["now"].as_u64().unwrap(),
                    c["keyRole"].as_str().unwrap_or("current"),
                    text(&f, "keyId"),
                    &key
                ),
                c["accepted"].as_bool().unwrap(),
                "{}",
                text(c, "name")
            );
        }
        for c in f["requestCases"].as_array().unwrap() {
            assert_eq!(
                request(text(c, "raw")),
                c["accepted"].as_bool().unwrap(),
                "{}",
                text(c, "name")
            );
        }
        for c in f["responseCases"].as_array().unwrap() {
            assert_eq!(
                response(text(c, "raw")),
                c["accepted"].as_bool().unwrap(),
                "{}",
                text(c, "name")
            );
        }
    }
}
