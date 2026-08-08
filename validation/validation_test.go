// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package validation_test

import (
	"testing"

	gamev1 "github.com/atrinik/protocol/gen/go/atrinik/game/v1"
	"github.com/atrinik/protocol/validation"
)

func TestIdentityAndCoordinateBounds(t *testing.T) {
	validID := make([]byte, 16)
	validID[0] = 1
	valid := &gamev1.TilePosition{
		MapInstance:   &gamev1.MapInstanceId{Value: validID},
		X:             validation.MinimumCoordinate,
		Y:             validation.MaximumCoordinate,
		PhysicalLevel: -32,
		Surface:       255,
	}
	if err := validation.TilePosition(valid); err != nil {
		t.Fatal(err)
	}

	valid.X--
	if err := validation.TilePosition(valid); err == nil {
		t.Fatal("coordinate below the minimum was accepted")
	}
}

func TestTextBounds(t *testing.T) {
	if err := validation.Text("safe", 4); err != nil {
		t.Fatal(err)
	}
	if err := validation.Text("nul\x00value", validation.MaximumTextBytes); err == nil {
		t.Fatal("NUL-containing text was accepted")
	}
	if err := validation.Text(string([]byte{0xff}), validation.MaximumTextBytes); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
}

func TestStableIDsRejectPathConfusion(t *testing.T) {
	valid := &gamev1.ContentId{Namespace: "world.maps", Value: "clearhaven/level-1"}
	if err := validation.ContentID(valid); err != nil {
		t.Fatal(err)
	}

	invalid := []struct {
		namespace string
		value     string
	}{
		{"world/maps", "clearhaven"},
		{"world", "/clearhaven"},
		{"world", "clearhaven/"},
		{"world", "clearhaven//level-1"},
		{"world", "clearhaven/../private"},
		{"world", ".hidden"},
	}
	for _, value := range invalid {
		candidate := &gamev1.ContentId{Namespace: value.namespace, Value: value.value}
		if err := validation.ContentID(candidate); err == nil {
			t.Fatalf("unsafe content ID accepted: %q:%q", value.namespace, value.value)
		}
	}

	resource := &gamev1.ResourceId{Namespace: "graphics", Value: "tiles/floor-1"}
	if err := validation.ResourceID(resource); err != nil {
		t.Fatal(err)
	}
}
