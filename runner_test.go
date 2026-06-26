// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"gopkg.in/yaml.v2"
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

// TestTopLevelSkippedFails locks in the behavior that a policy
// whose top-level result is StatusSkipped is reported as a test
// failure (because policy-bot's server translates that into a
// failing "All rules were skipped" GitHub status check). The
// runner can be opted out by asserting `expect.status: skipped`.
func TestTopLevelSkippedFails(t *testing.T) {
	// A policy with a single rule that only fires on changes under
	// foo/. Any PR not touching foo/ resolves to StatusSkipped at
	// the top level.
	const policyYAML = `
policy:
  approval:
    - only foo changes need a review
approval_rules:
  - name: only foo changes need a review
    if:
      changed_files:
        paths: ["^foo/"]
    requires:
      count: 1
      teams: ["org/team"]
`
	var cfg policy.Config
	if err := yaml.UnmarshalStrict([]byte(policyYAML), &cfg); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	eval, err := policy.ParsePolicy(&cfg, nil)
	if err != nil {
		t.Fatalf("ParsePolicy: %v", err)
	}

	tests := &TestConfig{
		Teams: map[string][]string{"org/team": {"alice"}},
		Tests: []TestCase{
			{
				Name: "no_expectation_set_should_fail",
				PR:   PullRequest{Author: "alice", ChangedFiles: []File{{Filename: "README.md"}}},
				// Expect intentionally empty.
			},
			{
				Name:   "explicit_opt_in_should_pass",
				PR:     PullRequest{Author: "alice", ChangedFiles: []File{{Filename: "README.md"}}},
				Expect: Expectations{Status: "skipped"},
			},
		},
	}

	var buf bytes.Buffer
	failed := Run(eval, tests, &buf)
	if failed != 1 {
		t.Errorf("failed=%d, want 1\noutput:\n%s", failed, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "FAIL  no_expectation_set_should_fail") {
		t.Errorf("expected first test to FAIL; got:\n%s", out)
	}
	if !strings.Contains(out, "PASS  explicit_opt_in_should_pass") {
		t.Errorf("expected explicit opt-in to PASS; got:\n%s", out)
	}
	// Also: the synthetic policy's evaluator does in fact return Skipped.
	// Guard against an upstream API change that would invalidate this test.
	r := eval.Evaluate(context.Background(), buildContext(tests, tests.Tests[0]))
	if r.Status != common.StatusSkipped {
		t.Errorf("evaluator returned %s; expected StatusSkipped (upstream change?)", r.Status)
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
