// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"encoding/base64"
	"testing"
)

func decodeBase64(t testing.TB, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
