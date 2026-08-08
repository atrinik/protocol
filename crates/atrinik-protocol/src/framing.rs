// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

use std::fmt;

pub const MAXIMUM_GAMEPLAY_FRAME: u32 = 1 << 20;
pub const MAXIMUM_RESOURCE_FRAME: u32 = 4 << 20;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Error {
    Empty,
    TooLarge,
    LengthOverflow,
    NonCanonicalLength,
    Truncated,
}

impl fmt::Display for Error {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(formatter, "invalid Game Protocol frame: {self:?}")
    }
}

impl std::error::Error for Error {}

pub fn decode(input: &[u8], limit: u32) -> Result<(&[u8], &[u8]), Error> {
    let (length, prefix_bytes) = decode_length(input)?;
    if length == 0 {
        return Err(Error::Empty);
    }
    if length > limit {
        return Err(Error::TooLarge);
    }
    let payload_end = prefix_bytes
        .checked_add(length as usize)
        .ok_or(Error::LengthOverflow)?;
    if input.len() < payload_end {
        return Err(Error::Truncated);
    }
    Ok((&input[prefix_bytes..payload_end], &input[payload_end..]))
}

pub fn encode(payload: &[u8], limit: u32) -> Result<Vec<u8>, Error> {
    if payload.is_empty() {
        return Err(Error::Empty);
    }
    let length = u32::try_from(payload.len()).map_err(|_| Error::TooLarge)?;
    if length > limit {
        return Err(Error::TooLarge);
    }
    let mut output = Vec::with_capacity(5 + payload.len());
    encode_length(length, &mut output);
    output.extend_from_slice(payload);
    Ok(output)
}

fn decode_length(input: &[u8]) -> Result<(u32, usize), Error> {
    let mut value = 0_u32;
    for index in 0..5_usize {
        let current = *input.get(index).ok_or(Error::Truncated)?;
        if index == 4 && current > 0x0f {
            return Err(Error::LengthOverflow);
        }
        value |= u32::from(current & 0x7f) << (7 * index);
        if current & 0x80 == 0 {
            if index > 0 && current == 0 {
                return Err(Error::NonCanonicalLength);
            }
            return Ok((value, index + 1));
        }
    }
    Err(Error::LengthOverflow)
}

fn encode_length(mut value: u32, output: &mut Vec<u8>) {
    while value >= 0x80 {
        output.push((value as u8) | 0x80);
        value >>= 7;
    }
    output.push(value as u8);
}

#[cfg(test)]
mod tests {
    use prost::Message;

    use super::{Error, MAXIMUM_GAMEPLAY_FRAME, decode, encode};
    use crate::game::v1::{ControlEnvelope, Ping, control_envelope};

    #[test]
    fn golden_control_envelope() {
        let payload = ControlEnvelope {
            sequence: 1,
            payload: Some(control_envelope::Payload::Ping(Ping { nonce: 1 })),
        }
        .encode_to_vec();
        assert_eq!(payload, [0x08, 0x01, 0x2a, 0x02, 0x08, 0x01]);
        let encoded = encode(&payload, MAXIMUM_GAMEPLAY_FRAME).unwrap();
        assert_eq!(encoded, [0x06, 0x08, 0x01, 0x2a, 0x02, 0x08, 0x01]);
        assert_eq!(
            decode(&encoded, MAXIMUM_GAMEPLAY_FRAME),
            Ok((&payload[..], &[][..]))
        );
    }

    #[test]
    fn rejects_invalid_lengths_without_payload() {
        assert_eq!(decode(&[0], 1024), Err(Error::Empty));
        assert_eq!(decode(&[0x86, 0], 1024), Err(Error::NonCanonicalLength));
        assert_eq!(
            decode(&[0xff, 0xff, 0xff, 0xff, 0x1f], 1024),
            Err(Error::LengthOverflow)
        );
        assert_eq!(decode(&[0x81, 0x08], 1024), Err(Error::TooLarge));
        assert_eq!(decode(&[2, 1], 1024), Err(Error::Truncated));
    }
}
