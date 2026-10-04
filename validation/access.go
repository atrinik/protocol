// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package validation

import (
	"bytes"
	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
	access "github.com/atrinik/protocol/metaserver/v2"
)

// AccessServerHello validates the mandatory GP1 1.1 access policy. Callers still
// enforce connection ordering and all other ServerHello bounds transactionally.
func AccessServerHello(value *gamev1.ServerHello) error {
	if value == nil || value.Version == nil || value.Version.Major != 1 || value.Version.Minor < 1 ||
		(value.AccessPolicy != gamev1.AccessPolicy_ACCESS_POLICY_OPEN && value.AccessPolicy != gamev1.AccessPolicy_ACCESS_POLICY_PROTECTED) {
		return ErrInvalidBound
	}
	found := false
	for _, capability := range value.Capabilities {
		if capability == gamev1.Capability_CAPABILITY_ACCESS_TOKENS_V1 {
			if found {
				return ErrInvalidBound
			}
			found = true
		}
	}
	if !found {
		return ErrInvalidBound
	}
	return nil
}

// AccessAuth checks exact wire code shape and binding to the negotiated session.
// It performs no authorization and never retains the credential buffer.
func AccessAuth(value *gamev1.AccessAuth, sessionID []byte) error {
	if value == nil || value.SessionId == nil || Opaque16(sessionID) != nil ||
		!bytes.Equal(value.SessionId.Value, sessionID) || !access.CanonicalAccessCode(value.Code) {
		return ErrInvalidBound
	}
	return nil
}

func AccessResult(value *gamev1.AccessResult) error {
	if value == nil || (value.Status != gamev1.AccessStatus_ACCESS_STATUS_ACCEPTED && value.Status != gamev1.AccessStatus_ACCESS_STATUS_UNAVAILABLE) {
		return ErrInvalidBound
	}
	return nil
}
