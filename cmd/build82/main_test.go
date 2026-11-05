// Copyright (C) 2026  OITO2
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/selfupdate"
)

// mustParseSelfUpdateFlags parses `args` and fails the test on a usage error.
func mustParseSelfUpdateFlags(t *testing.T, args []string) selfupdate.RunOptions {
	t.Helper()
	o, _, err := parseSelfUpdateFlags(args)
	if err != nil {
		t.Fatalf("unexpected usage error: %v", err)
	}
	return o
}

// TestParseSelfUpdateFlags_Defaults verifies the defaults: no check, no confirmation skip, stable channel.
func TestParseSelfUpdateFlags_Defaults(t *testing.T) {
	opts := mustParseSelfUpdateFlags(t, nil)
	if opts.Check {
		t.Error("expected check=false by default")
	}
	if opts.Yes {
		t.Error("expected yes=false by default")
	}
	if opts.Channel != "stable" {
		t.Errorf("expected default channel 'stable', got %q", opts.Channel)
	}
}

// TestParseSelfUpdateFlags_CheckAndYes verifies that --check and -y set their options.
func TestParseSelfUpdateFlags_CheckAndYes(t *testing.T) {
	opts := mustParseSelfUpdateFlags(t, []string{"--check", "-y"})
	if !opts.Check {
		t.Error("expected check=true")
	}
	if !opts.Yes {
		t.Error("expected yes=true")
	}
}

// TestParseSelfUpdateFlags_YesLongForm verifies that --yes behaves like -y.
func TestParseSelfUpdateFlags_YesLongForm(t *testing.T) {
	opts := mustParseSelfUpdateFlags(t, []string{"--yes"})
	if !opts.Yes {
		t.Error("expected yes=true from --yes")
	}
}

// TestParseSelfUpdateFlags_Channel verifies that --channel sets the release channel.
func TestParseSelfUpdateFlags_Channel(t *testing.T) {
	opts := mustParseSelfUpdateFlags(t, []string{"--channel", "beta"})
	if opts.Channel != "beta" {
		t.Errorf("expected channel 'beta', got %q", opts.Channel)
	}
}

// TestParseUninstallArgs_TargetOnly verifies that a lone target is returned without purge.
func TestParseUninstallArgs_TargetOnly(t *testing.T) {
	target, purge, err := parseUninstallArgs([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if target != "claude" {
		t.Errorf("expected target 'claude', got %q", target)
	}
	if purge {
		t.Error("expected purge=false by default")
	}
}

// TestParseUninstallArgs_PurgeOnly verifies that --purge alone yields no target.
func TestParseUninstallArgs_PurgeOnly(t *testing.T) {
	target, purge, err := parseUninstallArgs([]string{"--purge"})
	if err != nil {
		t.Fatal(err)
	}
	if target != "" {
		t.Errorf("expected no target, got %q", target)
	}
	if !purge {
		t.Error("expected purge=true")
	}
}

// TestParseUninstallArgs_TargetAndPurgeAnyOrder verifies that --purge may precede the target.
func TestParseUninstallArgs_TargetAndPurgeAnyOrder(t *testing.T) {
	target, purge, err := parseUninstallArgs([]string{"--purge", "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if target != "cursor" {
		t.Errorf("expected target 'cursor', got %q", target)
	}
	if !purge {
		t.Error("expected purge=true")
	}
}

// TestParseUninstallArgs_NoArgs verifies that no arguments yield an empty target and no purge.
func TestParseUninstallArgs_NoArgs(t *testing.T) {
	target, purge, err := parseUninstallArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if target != "" || purge {
		t.Errorf("expected empty target and purge=false, got target=%q purge=%v", target, purge)
	}
}

// TestParseServeArgs verifies that the stdio server accepts no argument but a trailing "--", and
// that the flags of the removed HTTP mode are usage errors that say so.
func TestParseServeArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"--"}} {
		if err := parseServeArgs(args); err != nil {
			t.Errorf("parseServeArgs(%q) = %v, want nil", args, err)
		}
	}
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--bogus"}, `unknown argument "--bogus"`},
		{[]string{"extra"}, `unknown argument "extra"`},
		{[]string{"--", "x"}, `unexpected argument "x"`},
		{[]string{"--http"}, "the HTTP mode was removed in v1.2.0"},
		{[]string{"--port", "8080"}, "the HTTP mode was removed in v1.2.0"},
		{[]string{"--host", "0.0.0.0"}, "the HTTP mode was removed in v1.2.0"},
		{[]string{"--token", "x"}, "the HTTP mode was removed in v1.2.0"},
		{[]string{"--allowed-host", "a"}, "the HTTP mode was removed in v1.2.0"},
	}
	for _, tt := range tests {
		if err := parseServeArgs(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parseServeArgs(%q) = %v, want it to contain %q", tt.args, err, tt.want)
		}
	}
}

// TestParseSelfUpdateFlags_UsageErrors verifies that unknown arguments, a missing --channel value,
// --rollback combined with another flag, and --require-signature with --check are rejected.
func TestParseSelfUpdateFlags_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--bogus"}, {"extra"}, {"--channel"}, {"--check", "--channel"},
		{"--rollback", "--check"}, {"--yes", "--rollback"}, {"--rollback", "--require-signature"},
		{"--rollback", "--channel", "stable"}, {"--check", "--require-signature"},
	} {
		if _, _, err := parseSelfUpdateFlags(args); err == nil {
			t.Errorf("parseSelfUpdateFlags(%v) expected an error", args)
		}
	}
}

// TestParseSelfUpdateFlags_RollbackAndRequireSignature verifies that a lone --rollback is reported
// as a rollback and that --require-signature sets its option.
func TestParseSelfUpdateFlags_RollbackAndRequireSignature(t *testing.T) {
	if _, rollback, err := parseSelfUpdateFlags([]string{"--rollback"}); err != nil || !rollback {
		t.Errorf("--rollback: got rollback=%v, err=%v", rollback, err)
	}
	opts, rollback, err := parseSelfUpdateFlags([]string{"--require-signature", "-y"})
	if err != nil || rollback || !opts.RequireSignature || !opts.Yes {
		t.Errorf("--require-signature -y: got %+v, rollback=%v, err=%v", opts, rollback, err)
	}
}

// TestParseInstallArgs verifies the optional target and the rejection of flags and extra arguments.
func TestParseInstallArgs(t *testing.T) {
	if got, err := parseInstallArgs(nil); err != nil || got != "" {
		t.Errorf("no args: got %q, %v", got, err)
	}
	if got, err := parseInstallArgs([]string{"claude"}); err != nil || got != "claude" {
		t.Errorf("target: got %q, %v", got, err)
	}
	for _, args := range [][]string{{"--bogus"}, {"claude", "cursor"}, {"--purge"}} {
		if _, err := parseInstallArgs(args); err == nil {
			t.Errorf("parseInstallArgs(%v) expected an error", args)
		}
	}
}

// TestParseUninstallArgs_UsageErrors verifies that unknown flags and extra targets are rejected.
func TestParseUninstallArgs_UsageErrors(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {"claude", "cursor"}} {
		if _, _, err := parseUninstallArgs(args); err == nil {
			t.Errorf("parseUninstallArgs(%v) expected an error", args)
		}
	}
}

// TestParseTargetArgs_DoubleDashMakesEverythingPositional verifies that after "--" an argument
// starting with "-" is taken as the target, and that --purge is still recognized before it.
func TestParseTargetArgs_DoubleDashMakesEverythingPositional(t *testing.T) {
	target, purge, err := parseUninstallArgs([]string{"--purge", "--", "-odd"})
	if err != nil || target != "-odd" || !purge {
		t.Errorf("got target=%q purge=%v err=%v", target, purge, err)
	}
	if target, purge, err := parseUninstallArgs([]string{"--", "--purge"}); err != nil || target != "--purge" || purge {
		t.Errorf("--purge after --: got target=%q purge=%v err=%v", target, purge, err)
	}
	if _, err := parseInstallArgs([]string{"--", "a", "b"}); err == nil {
		t.Error("expected a second positional argument after -- to be rejected")
	}
}

// TestParseSelfUpdateAndServeFlags_DoubleDash verifies that a trailing "--" is accepted and that
// anything after it is an unexpected argument, since neither takes a positional argument.
func TestParseSelfUpdateAndServeFlags_DoubleDash(t *testing.T) {
	if _, _, err := parseSelfUpdateFlags([]string{"--yes", "--"}); err != nil {
		t.Errorf("self-update --yes --: unexpected error %v", err)
	}
	if _, _, err := parseSelfUpdateFlags([]string{"--", "--check"}); err == nil {
		t.Error("self-update -- --check: expected an error")
	}
	if err := parseServeArgs([]string{"--"}); err != nil {
		t.Errorf("--: unexpected error %v", err)
	}
	if err := parseServeArgs([]string{"--", "x"}); err == nil {
		t.Error("-- x: expected an error")
	}
}

// TestWantsHelp verifies that -h/--help count only before "--".
func TestWantsHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--help"}, true},
		{[]string{"claude", "-h"}, true},
		{[]string{"--", "--help"}, false},
		{[]string{"claude"}, false},
		{nil, false},
	} {
		if got := wantsHelp(tc.args); got != tc.want {
			t.Errorf("wantsHelp(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
