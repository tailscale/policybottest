// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestTestdata runs the example .policy.yml / .policy-test.yml pair
// under testdata/. It catches regressions in the runner itself and
// doubles as a smoke test that the upstream policy-bot evaluator
// API we depend on hasn't shifted under us.
func TestTestdata(t *testing.T) {
	cases := []struct {
		name       string
		policy     string
		tests      string
		wantFailed int
	}{
		{
			name:   "tailscale.com",
			policy: "testdata/tailscale.com.policy.yml",
			tests:  "testdata/tailscale.com.policy-test.yml",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eval, err := LoadPolicy(filepath.FromSlash(tc.policy))
			if err != nil {
				t.Fatalf("LoadPolicy: %v", err)
			}
			cfg, err := LoadTestConfig(filepath.FromSlash(tc.tests))
			if err != nil {
				t.Fatalf("LoadTestConfig: %v", err)
			}
			var buf bytes.Buffer
			failed := Run(eval, cfg, &buf)
			if failed != tc.wantFailed {
				t.Errorf("failed=%d, want %d\noutput:\n%s",
					failed, tc.wantFailed, buf.String())
			}
			// Sanity-check that every test actually ran (PASS or FAIL).
			for _, c := range cfg.Tests {
				if !strings.Contains(buf.String(), c.Name) {
					t.Errorf("test %q is missing from runner output", c.Name)
				}
			}
		})
	}
}

func TestInvertMembership(t *testing.T) {
	got := invertMembership(map[string][]string{
		"tailscale/dev":                    {"alice", "bob"},
		"tailscale/control-protocol-owners": {"alice"},
	})
	want := map[string][]string{
		"alice": {"tailscale/control-protocol-owners", "tailscale/dev"},
		"bob":   {"tailscale/dev"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d users, want %d: %v", len(got), len(want), got)
	}
	for user, wantTeams := range want {
		gotTeams := got[user]
		if !sameStrings(gotTeams, wantTeams) {
			t.Errorf("user %q: got teams %v, want %v", user, gotTeams, wantTeams)
		}
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
