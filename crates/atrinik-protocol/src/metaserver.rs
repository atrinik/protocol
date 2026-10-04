// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

pub mod v1 {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/generated/atrinik/metaserver/v1/atrinik.metaserver.v1.rs"
    ));
}

pub mod access;
pub mod directory;

/// Current access-token directory contracts; v1 is historical only.
pub mod v2 {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/generated/atrinik/metaserver/v2/atrinik.metaserver.v2.rs"
    ));
}

pub mod directory_v2;
