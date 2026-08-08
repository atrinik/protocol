// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

// Package validation enforces Game Protocol 1 common semantic bounds. Domain
// consumers remain responsible for authorization and state validation.
package validation

import (
	"errors"
	"strings"
	"unicode/utf8"

	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
)

const (
	MaximumTextBytes        = 4096
	MaximumSafeMessageBytes = 512
	MinimumCoordinate       = -1_048_576
	MaximumCoordinate       = 1_048_575
)

var (
	ErrInvalidBound = errors.New("game protocol value violates a semantic bound")
)

func Opaque16(value []byte) error {
	if len(value) != 16 || allZero(value) {
		return ErrInvalidBound
	}
	return nil
}

func Digest(value []byte) error {
	if len(value) != 32 {
		return ErrInvalidBound
	}
	return nil
}

func ContentID(value *gamev1.ContentId) error {
	if value == nil || !stableID(value.Namespace, value.Value) {
		return ErrInvalidBound
	}
	return nil
}

func ResourceID(value *gamev1.ResourceId) error {
	if value == nil || !stableID(value.Namespace, value.Value) {
		return ErrInvalidBound
	}
	return nil
}

func TilePosition(value *gamev1.TilePosition) error {
	if value == nil || value.MapInstance == nil ||
		Opaque16(value.MapInstance.Value) != nil ||
		value.X < MinimumCoordinate || value.X > MaximumCoordinate ||
		value.Y < MinimumCoordinate || value.Y > MaximumCoordinate ||
		value.PhysicalLevel < -32 || value.PhysicalLevel > 31 ||
		value.Surface > 255 {
		return ErrInvalidBound
	}
	return nil
}

func Text(value string, maximum int) error {
	if maximum < 0 || len(value) > maximum || !utf8.ValidString(value) {
		return ErrInvalidBound
	}
	for _, current := range value {
		if current == 0 {
			return ErrInvalidBound
		}
	}
	return nil
}

func allZero(value []byte) bool {
	var combined byte
	for _, current := range value {
		combined |= current
	}
	return combined == 0
}

func stableID(namespace string, value string) bool {
	if len(namespace) < 1 || len(namespace) > 32 ||
		len(value) < 1 || len(value) > 160 || !identifierSegment(namespace) {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if !identifierSegment(segment) {
			return false
		}
	}
	return true
}

func identifierSegment(value string) bool {
	if len(value) == 0 || !lowerAlphanumeric(value[0]) ||
		!lowerAlphanumeric(value[len(value)-1]) {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		current := value[index]
		if !lowerAlphanumeric(current) && current != '.' && current != '_' && current != '-' {
			return false
		}
	}
	return true
}

func lowerAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}
