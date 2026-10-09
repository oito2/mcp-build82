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

// mustParseServeFlags parses `args` and fails the test on a usage error.
func mustParseServeFlags(t *testing.T, args []string) serveFlags {
	t.Helper()
	f, err := parseServeFlags(args)
	if err != nil {
		t.Fatalf("unexpected usage error: %v", err)
	}
	return f
}

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

// TestParseServeFlags_Defaults verifies the default mode, port, host and token.
func TestParseServeFlags_Defaults(t *testing.T) {
	f := mustParseServeFlags(t, nil)
	if f.http {
		t.Error("expected http=false by default")
	}
	if f.port != 3000 {
		t.Errorf("expected default port 3000, got %d", f.port)
	}
	if f.host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %q", f.host)
	}
	if f.token != "" {
		t.Errorf("expected empty default token, got %q", f.token)
	}
}

// TestParseServeFlags_HttpAndPort verifies that --http and --port are parsed.
func TestParseServeFlags_HttpAndPort(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http", "--port", "8080"})
	if !f.http {
		t.Error("expected http=true")
	}
	if f.port != 8080 {
		t.Errorf("expected port 8080, got %d", f.port)
	}
}

// TestParseServeFlags_InvalidPortKeepsDefault verifies that a non-numeric port is ignored.
func TestParseServeFlags_InvalidPortKeepsDefault(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http", "--port", "not-a-number"})
	if f.port != 3000 {
		t.Errorf("expected invalid port to keep default 3000, got %d", f.port)
	}
}

// TestParseServeFlags_PortOutOfRangeKeepsDefault verifies that an out-of-range port is ignored.
func TestParseServeFlags_PortOutOfRangeKeepsDefault(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http", "--port", "70000"})
	if f.port != 3000 {
		t.Errorf("expected out-of-range port to keep default 3000, got %d", f.port)
	}
}

// TestParseServeFlags_RepeatableAllowedHost verifies that --allowed-host values accumulate in order.
func TestParseServeFlags_RepeatableAllowedHost(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http", "--allowed-host", "a.example.com", "--allowed-host", "b.example.com"})
	if len(f.allowedHosts) != 2 || f.allowedHosts[0] != "a.example.com" || f.allowedHosts[1] != "b.example.com" {
		t.Errorf("expected 2 allowed hosts appended in order, got %v", f.allowedHosts)
	}
}

// TestParseServeFlags_HostAndToken verifies that --host and --token are parsed and tokenSet is recorded.
func TestParseServeFlags_HostAndToken(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http", "--host", "0.0.0.0", "--token", "secret123"})
	if f.host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %q", f.host)
	}
	if f.token != "secret123" {
		t.Errorf("expected token secret123, got %q", f.token)
	}
	if !f.tokenSet {
		t.Error("expected tokenSet=true when --token is passed")
	}
}

// TestParseServeFlags_TokenNotPassed_TokenSetFalse verifies that tokenSet stays false without --token.
func TestParseServeFlags_TokenNotPassed_TokenSetFalse(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--http"})
	if f.tokenSet {
		t.Error("expected tokenSet=false when --token is not passed at all")
	}
	if f.token != "" {
		t.Errorf("expected empty token, got %q", f.token)
	}
}

// TestParseServeFlags_TokenExplicitEmpty_TokenSetTrue verifies that an explicit empty --token is recorded as set.
func TestParseServeFlags_TokenExplicitEmpty_TokenSetTrue(t *testing.T) {
	// An explicit `--token ""` (for example from an empty shell variable) must be distinguishable
	// from an omitted flag: parseServeFlags records that the flag was passed, and resolveToken
	// turns that into an error.
	f := mustParseServeFlags(t, []string{"--http", "--token", ""})
	if !f.tokenSet {
		t.Error("expected tokenSet=true when --token is passed with an explicit empty value")
	}
	if f.token != "" {
		t.Errorf("expected empty token value, got %q", f.token)
	}
}

// TestResolveToken_NothingConfigured_ReturnsEmptyNoError verifies that no token source yields an empty token and no error.
func TestResolveToken_NothingConfigured_ReturnsEmptyNoError(t *testing.T) {
	// Neither --token nor BUILD82_TOKEN is set: the only configuration that disables auth.
	token, err := resolveToken(serveFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

// TestResolveToken_CliTokenTakesPrecedenceOverEnv verifies that --token wins over the environment variable.
func TestResolveToken_CliTokenTakesPrecedenceOverEnv(t *testing.T) {
	t.Setenv(buildTokenEnvVar, "from-env")
	token, err := resolveToken(serveFlags{token: "from-cli", tokenSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "from-cli" {
		t.Errorf("expected --token to take precedence over BUILD82_TOKEN, got %q", token)
	}
}

// TestResolveToken_EnvVarFallback_WhenFlagNotPassed verifies that the environment variable is used without --token.
func TestResolveToken_EnvVarFallback_WhenFlagNotPassed(t *testing.T) {
	t.Setenv(buildTokenEnvVar, "from-env")
	token, err := resolveToken(serveFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "from-env" {
		t.Errorf("expected BUILD82_TOKEN fallback, got %q", token)
	}
}

// TestResolveToken_ExplicitEmptyCliToken_IsConfigError verifies that an explicit empty --token is
// a configuration error rather than silently disabling authentication.
func TestResolveToken_ExplicitEmptyCliToken_IsConfigError(t *testing.T) {
	_, err := resolveToken(serveFlags{token: "", tokenSet: true})
	if err == nil {
		t.Fatal("expected an error for an explicit empty --token")
	}
}

// TestResolveToken_ExplicitEmptyEnvToken_IsConfigError verifies the same for BUILD82_TOKEN set to an
// empty string, as opposed to unset.
func TestResolveToken_ExplicitEmptyEnvToken_IsConfigError(t *testing.T) {
	t.Setenv(buildTokenEnvVar, "")
	_, err := resolveToken(serveFlags{})
	if err == nil {
		t.Fatal("expected an error for an explicit empty BUILD82_TOKEN")
	}
}

// TestParseServeFlags_UsageErrors verifies that unknown arguments, missing values and server-only
// flags without --http are rejected.
func TestParseServeFlags_UsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--http", "--bogus"}, "unknown argument"},
		{"stray positional", []string{"--http", "extra"}, "unknown argument"},
		{"port without http", []string{"--port", "8080"}, "--port only applies with --http"},
		{"host without http", []string{"--host", "0.0.0.0"}, "--host only applies with --http"},
		{"token without http", []string{"--token", "x"}, "--token only applies with --http"},
		{"allowed-host without http", []string{"--allowed-host", "a"}, "--allowed-host only applies with --http"},
		{"missing port value", []string{"--http", "--port"}, "--port requires a value"},
		{"missing host value", []string{"--http", "--host"}, "--host requires a value"},
		{"missing token value", []string{"--http", "--token"}, "--token requires a value"},
		{"missing allowed-host value", []string{"--http", "--allowed-host"}, "--allowed-host requires a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseServeFlags(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parseServeFlags(%v) error = %v, want it to contain %q", tt.args, err, tt.want)
			}
		})
	}
}

// TestParseServeFlags_ServerFlagsBeforeHTTP verifies that flag order does not matter.
func TestParseServeFlags_ServerFlagsBeforeHTTP(t *testing.T) {
	f := mustParseServeFlags(t, []string{"--port", "8080", "--http"})
	if !f.http || f.port != 8080 {
		t.Errorf("unexpected flags: %+v", f)
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
	if _, err := parseServeFlags([]string{"--http", "--"}); err != nil {
		t.Errorf("--http --: unexpected error %v", err)
	}
	if _, err := parseServeFlags([]string{"--", "x"}); err == nil {
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
