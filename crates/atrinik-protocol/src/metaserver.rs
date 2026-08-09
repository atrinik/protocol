// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

pub mod v1 {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../gen/rust/atrinik/metaserver/v1/atrinik.metaserver.v1.rs"
    ));
}

pub mod directory;
