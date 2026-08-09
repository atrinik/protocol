// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

#![forbid(unsafe_code)]

pub mod framing;
pub mod metaserver;
pub mod validation;

pub mod game {
    pub mod v1 {
        include!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../../gen/rust/atrinik/game/v1/atrinik.game.v1.rs"
        ));
    }
}
