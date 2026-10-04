// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver

import (
	"crypto/sha256"
	"errors"
	"strings"
)

const AccessCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
const AccessCodeLength = 16

var ErrInvalidAccessCode = errors.New("invalid access code")

// CanonicalAccessCode validates wire input. It never returns rejected input.
func CanonicalAccessCode(code []byte) bool {
	if len(code) != AccessCodeLength {
		return false
	}
	for _, value := range code {
		if !strings.ContainsRune(AccessCodeAlphabet, rune(value)) {
			return false
		}
	}
	return true
}

// AccessRouteCapability derives a discovery capability, not a game credential.
func AccessRouteCapability(code []byte) ([32]byte, error) {
	if !CanonicalAccessCode(code) {
		return [32]byte{}, ErrInvalidAccessCode
	}
	h := sha256.New()
	h.Write([]byte("atrinik-access-route-v1\x00"))
	h.Write(code)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

func AccessRouteIndex(capability [32]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("atrinik-access-index-v1\x00"))
	h.Write(capability[:])
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func AccessVerifier(serverID [32]byte, code []byte) ([32]byte, error) {
	if !CanonicalAccessCode(code) {
		return [32]byte{}, ErrInvalidAccessCode
	}
	h := sha256.New()
	h.Write([]byte("atrinik-access-join-v1\x00"))
	h.Write(serverID[:])
	h.Write(code)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}
