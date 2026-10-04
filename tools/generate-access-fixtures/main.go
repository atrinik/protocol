// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

// Command generate-access-fixtures regenerates synthetic signed publisher vectors.
// It uses only the pinned Go standard library and the public test scalar 1.
package main

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	meta "github.com/atrinik/protocol/metaserver/v2"
)

const certificate = "MIIBOjCB4KADAgECAgID6TAKBggqhkjOPQQDAjAmMSQwIgYDVQQDDBtBdHJpbmlrIGFjY2VzcyBmaXh0dXJlIG9ubHkwHhcNMjYwMTAxMDAwMDAwWhcNMzYwMTAxMDAwMDAwWjAmMSQwIgYDVQQDDBtBdHJpbmlrIGFjY2VzcyBmaXh0dXJlIG9ubHkwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAARrF9Hy4SxCR/i85uVjpEDydwN9gS3rM6D0oTlF2JjClk/jQuL+Gn+bjufrSnwPnhYrzjNXazFezsu2QGg3v1H1MAoGCCqGSM49BAMCA0kAMEYCIQDuJYjSE1s0zA8WTnf+zwhLUj7HiAN3I4u9Se0dmU2jvAIhALgq0zfa5cvIFi8xBKYqCN8gNsxnhvb2qPHKe/pUq8DT"

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) map[string]any {
	data, err := os.ReadFile(path)
	must(err)
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	must(decoder.Decode(&result))
	return result
}
func replace(value any, replacer *strings.Replacer) any {
	switch value := value.(type) {
	case string:
		return replacer.Replace(value)
	case []any:
		for i := range value {
			value[i] = replace(value[i], replacer)
		}
	case map[string]any:
		for k, v := range value {
			value[k] = replace(v, replacer)
		}
	}
	return value
}
func sign(vector, root map[string]any, profile meta.Profile, key *ecdsa.PrivateKey) {
	sequence, err := strconv.ParseUint(vector["sequence"].(string), 10, 64)
	must(err)
	rawNonce, err := hex.DecodeString(vector["nonce"].(string))
	must(err)
	if len(rawNonce) != 16 {
		panic("nonce width")
	}
	var nonce [16]byte
	copy(nonce[:], rawNonce)
	created, err := root["created"].(json.Number).Int64()
	must(err)
	components, err := meta.Build(meta.Parameters{Profile: profile, Authority: root["authority"].(string), ServerID: root["server_id"].(string), Sequence: sequence, Nonce: nonce, Created: created}, []byte(vector["body"].(string)))
	must(err)
	digest := sha256.Sum256([]byte(components.SignatureBase))
	// A nil randomness source explicitly selects RFC6979 in the pinned Go API.
	der, err := key.Sign(nil, digest[:], crypto.SHA256)
	must(err)
	var signature struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &signature)
	must(err)
	if len(rest) != 0 {
		panic("signature trailing bytes")
	}
	raw := make([]byte, 64)
	signature.R.FillBytes(raw[:32])
	signature.S.FillBytes(raw[32:])
	encoded := base64.StdEncoding.EncodeToString(raw)
	vector["content_digest"] = components.ContentDigest
	vector["signature_input"] = components.SignatureInput
	vector["signature_base"] = components.SignatureBase
	vector["signature_base64"] = encoded
	vector["signature_header"] = "atrinik=:" + encoded + ":"
	if vector["path"] != nil {
		vector["path"] = components.Path
	}
}
func output(path string, value map[string]any, check bool) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(value))
	if check {
		existing, err := os.ReadFile(path)
		must(err)
		if !bytes.Equal(existing, buffer.Bytes()) {
			panic("fixture drift: " + path)
		}
		return
	}
	must(os.WriteFile(path, buffer.Bytes(), 0644))
}
func main() {
	check := flag.Bool("check", false, "reject generated fixture drift without writing")
	flag.Parse()
	scalar := make([]byte, 32)
	scalar[31] = 1
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), scalar)
	must(err)
	der, err := base64.StdEncoding.DecodeString(certificate)
	must(err)
	identity := sha256.Sum256(der)
	serverID := hex.EncodeToString(identity[:])
	for _, classic := range []bool{true, false} {
		source, destination, profile := "fixtures/metaserver-game-publisher-v1.json", "fixtures/metaserver-game-publisher-v2.json", meta.GameProfile
		if classic {
			source, destination, profile = "fixtures/metaserver-publisher-v1.json", "fixtures/metaserver-classic-publisher-v3.json", meta.ClassicV3Profile
		}
		root := read(source)
		replacements := []string{root["certificate_der_base64"].(string), certificate, root["server_id"].(string), serverID, "passwordRequired", "accessRequired", "atrinik-game-publish-v2", "atrinik-game-publish-v99", "atrinik-game-publish-v1", "atrinik-game-publish-v2", "/v1/servers/", "/v2/servers/"}
		if classic {
			replacements = append(replacements, "atrinik-classic-publish-v1", "atrinik-classic-publish-v3", "/v1/classic/servers/", "/v3/classic/servers/")
		}
		replace(root, strings.NewReplacer(replacements...))
		sign(root, root, profile, key)
		if classic {
			for _, name := range []string{"heartbeat", "changed", "private", "reused_nonce", "stale"} {
				sign(root[name].(map[string]any), root, profile, key)
			}
			// The open publication can precede the protected root in one replay lineage.
			sequence, err := strconv.ParseUint(root["sequence"].(string), 10, 64)
			must(err)
			open := map[string]any{"body": strings.Replace(root["body"].(string), `"accessRequired":true`, `"accessRequired":false`, 1), "sequence": strconv.FormatUint(sequence-2, 10), "nonce": "140102030405060708090a0b0c0d0e0f"}
			sign(open, root, profile, key)
			root["open"] = open
			// Keep the cross-profile negative transcript internally digest-consistent.
			root["game_signature_base"] = strings.ReplaceAll(root["signature_base"].(string), root["path"].(string), root["game_path"].(string))
			root["game_signature_base"] = strings.ReplaceAll(root["game_signature_base"].(string), "atrinik-classic-publish-v3", "atrinik-game-publish-v2")
		}
		output(destination, root, *check)
		fmt.Println(destination)
	}
}
