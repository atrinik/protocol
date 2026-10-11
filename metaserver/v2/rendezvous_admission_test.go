// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

package metaserver_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const admissionMaxSeconds uint64 = 253402300799

var admissionToken = regexp.MustCompile(`^v1\.([A-Za-z0-9_-]{1,32})\.([1-9][0-9]{0,11})\.([0-9a-f]{64})\.([0-9a-f]{32})\.([0-9a-f]{64})$`)
var admissionHex = regexp.MustCompile(`^[0-9a-f]{64}$`)
var admissionSolution = regexp.MustCompile(`^[0-9a-f]{16}$`)
var admissionRequest = regexp.MustCompile(`^\{"schema":"atrinik-rendezvous-admission-v1","ticketHash":"([0-9a-f]{64})"\}$`)
var admissionResponse = regexp.MustCompile(`^\{"schema":"atrinik-rendezvous-admission-v1","challenge":"([^"\\]*)","difficulty":24,"expiresAt":"([1-9][0-9]{0,11})"\}$`)

type admissionCase struct {
	Name     string
	Now      uint64
	Accepted bool
	KeyRole  string
	Override map[string]string
}
type admissionJSONCase struct {
	Name     string
	Raw      string
	Accepted bool
}
type admissionFixture struct {
	Authority, ServerID, Ticket, TicketHash, KeyID, KeyHex, TokenPrefix, HmacInputHex, Mac, Token, Solution, WorkInputHex, WorkDigest, ChallengeRequest, ChallengeResponse string
	Synthetic                                                                                                                                                              bool
	Cases                                                                                                                                                                  []admissionCase
	RequestCases, ResponseCases                                                                                                                                            []admissionJSONCase
}

func admissionSHA(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}
func admissionSigning(authority, server, prefix string) []byte {
	return []byte("atrinik-rendezvous-admission-v1\x00" + authority + "\x00classic\x00" + server + "\x00" + prefix)
}
func admissionWork(token, solution string) []byte {
	return []byte("atrinik-rendezvous-work-v1\x00" + token + "\x00" + solution)
}
func admissionMAC(key, input []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(input)
	return mac.Sum(nil)
}
func admissionParseToken(token string) ([]string, uint64, bool) {
	fields := admissionToken.FindStringSubmatch(token)
	if fields == nil {
		return nil, 0, false
	}
	issued, err := strconv.ParseUint(fields[2], 10, 64)
	return fields, issued, err == nil && issued <= admissionMaxSeconds-60
}
func admissionVerify(token, solution, ticket, authority, server string, now uint64, keys map[string][]byte) bool {
	if len(token)+1+len(solution) > 256 || !admissionHex.MatchString(ticket) || !admissionHex.MatchString(server) || !admissionSolution.MatchString(solution) {
		return false
	}
	fields, issued, ok := admissionParseToken(token)
	if !ok || now < issued || now >= issued+60 {
		return false
	}
	binary, _ := hex.DecodeString(ticket)
	if fields[3] != admissionSHA(binary) {
		return false
	}
	key, exists := keys[fields[1]]
	if !exists {
		return false
	}
	mac, _ := hex.DecodeString(fields[5])
	prefix := token[:strings.LastIndexByte(token, '.')]
	if !hmac.Equal(mac, admissionMAC(key, admissionSigning(authority, server, prefix))) {
		return false
	}
	work := sha256.Sum256(admissionWork(token, solution))
	return work[0] == 0 && work[1] == 0 && work[2] == 0
}
func TestRendezvousAdmissionIndependentFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../fixtures/rendezvous-admission-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f admissionFixture
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(f.KeyHex)
	if err != nil || len(key) != 32 || !f.Synthetic {
		t.Fatal("synthetic key fixture")
	}
	ticket, _ := hex.DecodeString(f.Ticket)
	if admissionSHA(ticket) != f.TicketHash || admissionSHA([]byte(f.Ticket)) == f.TicketHash {
		t.Fatal("binary ticket hash")
	}
	signing := admissionSigning(f.Authority, f.ServerID, f.TokenPrefix)
	if hex.EncodeToString(signing) != f.HmacInputHex || hex.EncodeToString(admissionMAC(key, signing)) != f.Mac || f.TokenPrefix+"."+f.Mac != f.Token {
		t.Fatal("HMAC bytes")
	}
	work := admissionWork(f.Token, f.Solution)
	if hex.EncodeToString(work) != f.WorkInputHex || admissionSHA(work) != f.WorkDigest || !strings.HasPrefix(f.WorkDigest, "000000") {
		t.Fatal("work bytes")
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			value := map[string]string{"token": f.Token, "solution": f.Solution, "ticket": f.Ticket, "authority": f.Authority, "serverId": f.ServerID}
			for k, v := range c.Override {
				value[k] = v
			}
			keys := map[string][]byte{}
			if c.KeyRole == "" || c.KeyRole == "current" || c.KeyRole == "previous" {
				keys[f.KeyID] = key
			}
			if admissionVerify(value["token"], value["solution"], value["ticket"], value["authority"], value["serverId"], c.Now, keys) != c.Accepted {
				t.Fatal("acceptance mismatch")
			}
		})
	}
	for _, c := range f.RequestCases {
		t.Run("request/"+c.Name, func(t *testing.T) {
			if (len(c.Raw) <= 256 && admissionRequest.MatchString(c.Raw)) != c.Accepted {
				t.Fatal("request grammar mismatch")
			}
		})
	}
	for _, c := range f.ResponseCases {
		t.Run("response/"+c.Name, func(t *testing.T) {
			accepted := false
			m := admissionResponse.FindStringSubmatch(c.Raw)
			if len(c.Raw) <= 512 && m != nil {
				_, issued, ok := admissionParseToken(m[1])
				accepted = ok && strconv.FormatUint(issued+60, 10) == m[2]
			}
			if accepted != c.Accepted {
				t.Fatal("response grammar mismatch")
			}
		})
	}
}
