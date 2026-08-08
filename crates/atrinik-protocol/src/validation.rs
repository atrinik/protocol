// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

use crate::game::v1::{ContentId, ResourceId, TilePosition};

pub const MAXIMUM_TEXT_BYTES: usize = 4096;
pub const MAXIMUM_SAFE_MESSAGE_BYTES: usize = 512;
pub const MINIMUM_COORDINATE: i32 = -1_048_576;
pub const MAXIMUM_COORDINATE: i32 = 1_048_575;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InvalidBound;

pub fn opaque_16(value: &[u8]) -> Result<(), InvalidBound> {
    if value.len() != 16 || value.iter().all(|current| *current == 0) {
        return Err(InvalidBound);
    }
    Ok(())
}

pub fn digest(value: &[u8]) -> Result<(), InvalidBound> {
    (value.len() == 32).then_some(()).ok_or(InvalidBound)
}

pub fn content_id(value: &ContentId) -> Result<(), InvalidBound> {
    if !stable_id(&value.namespace, &value.value) {
        return Err(InvalidBound);
    }
    Ok(())
}

pub fn resource_id(value: &ResourceId) -> Result<(), InvalidBound> {
    if !stable_id(&value.namespace, &value.value) {
        return Err(InvalidBound);
    }
    Ok(())
}

pub fn tile_position(value: &TilePosition) -> Result<(), InvalidBound> {
    let map = value.map_instance.as_ref().ok_or(InvalidBound)?;
    opaque_16(&map.value)?;
    if !(MINIMUM_COORDINATE..=MAXIMUM_COORDINATE).contains(&value.x)
        || !(MINIMUM_COORDINATE..=MAXIMUM_COORDINATE).contains(&value.y)
        || !(-32..=31).contains(&value.physical_level)
        || value.surface > 255
    {
        return Err(InvalidBound);
    }
    Ok(())
}

pub fn text(value: &str, maximum: usize) -> Result<(), InvalidBound> {
    if value.len() > maximum || value.contains('\0') {
        return Err(InvalidBound);
    }
    Ok(())
}

fn stable_id(namespace: &str, value: &str) -> bool {
    (1..=32).contains(&namespace.len())
        && (1..=160).contains(&value.len())
        && identifier_segment(namespace)
        && value.split('/').all(identifier_segment)
}

fn identifier_segment(value: &str) -> bool {
    let bytes = value.as_bytes();
    let Some((first, remainder)) = bytes.split_first() else {
        return false;
    };
    let Some(last) = bytes.last() else {
        return false;
    };
    lower_alphanumeric(*first)
        && lower_alphanumeric(*last)
        && remainder
            .iter()
            .take(remainder.len().saturating_sub(1))
            .all(|current| lower_alphanumeric(*current) || matches!(current, b'.' | b'_' | b'-'))
}

fn lower_alphanumeric(value: u8) -> bool {
    value.is_ascii_lowercase() || value.is_ascii_digit()
}

#[cfg(test)]
mod tests {
    use bytes::Bytes;

    use super::{
        MAXIMUM_COORDINATE, MINIMUM_COORDINATE, content_id, opaque_16, text, tile_position,
    };
    use crate::game::v1::{ContentId, MapInstanceId, ResourceId, TilePosition};

    #[test]
    fn validates_identity_and_coordinate_edges() {
        let mut identifier = [0_u8; 16];
        identifier[0] = 1;
        assert_eq!(opaque_16(&identifier), Ok(()));

        let mut position = TilePosition {
            map_instance: Some(MapInstanceId {
                value: Bytes::copy_from_slice(&identifier),
            }),
            x: MINIMUM_COORDINATE,
            y: MAXIMUM_COORDINATE,
            physical_level: -32,
            surface: 255,
        };
        assert_eq!(tile_position(&position), Ok(()));
        position.x -= 1;
        assert!(tile_position(&position).is_err());
    }

    #[test]
    fn validates_text_and_stable_ids() {
        assert_eq!(text("safe", 4), Ok(()));
        assert!(text("nul\0value", 32).is_err());
        assert_eq!(
            content_id(&ContentId {
                namespace: "world".into(),
                value: "map/clearhaven".into(),
            }),
            Ok(())
        );
        assert!(
            content_id(&ContentId {
                namespace: "World".into(),
                value: "map/clearhaven".into(),
            })
            .is_err()
        );
        for value in [
            "/map/clearhaven",
            "map/clearhaven/",
            "map//clearhaven",
            "map/../private",
            ".hidden",
        ] {
            assert!(
                content_id(&ContentId {
                    namespace: "world".into(),
                    value: value.into(),
                })
                .is_err()
            );
        }
        assert_eq!(
            super::resource_id(&ResourceId {
                namespace: "graphics".into(),
                value: "tiles/floor-1".into(),
            }),
            Ok(())
        );
    }
}
