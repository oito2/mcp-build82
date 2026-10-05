// Copyright (C) 2026  oito2
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

import "testing"

func TestParseSelfUpdateFlags_Defaults(t *testing.T) {
	opts := parseSelfUpdateFlags(nil)
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

func TestParseSelfUpdateFlags_CheckAndYes(t *testing.T) {
	opts := parseSelfUpdateFlags([]string{"--check", "-y"})
	if !opts.Check {
		t.Error("expected check=true")
	}
	if !opts.Yes {
		t.Error("expected yes=true")
	}
}

func TestParseSelfUpdateFlags_YesLongForm(t *testing.T) {
	opts := parseSelfUpdateFlags([]string{"--yes"})
	if !opts.Yes {
		t.Error("expected yes=true from --yes")
	}
}

func TestParseSelfUpdateFlags_Channel(t *testing.T) {
	opts := parseSelfUpdateFlags([]string{"--channel", "beta"})
	if opts.Channel != "beta" {
		t.Errorf("expected channel 'beta', got %q", opts.Channel)
	}
}

func TestHasFlag_Present(t *testing.T) {
	if !hasFlag([]string{"--check", "--rollback"}, "--rollback") {
		t.Error("expected hasFlag to find --rollback among the args")
	}
}

func TestHasFlag_Absent(t *testing.T) {
	if hasFlag([]string{"--check", "--yes"}, "--rollback") {
		t.Error("expected hasFlag to report false when the flag isn't present")
	}
}

func TestHasFlag_EmptyArgs(t *testing.T) {
	if hasFlag(nil, "--rollback") {
		t.Error("expected hasFlag to report false for a nil args slice")
	}
}

func TestParseUninstallArgs_TargetOnly(t *testing.T) {
	target, purge := parseUninstallArgs([]string{"claude"})
	if target != "claude" {
		t.Errorf("expected target 'claude', got %q", target)
	}
	if purge {
		t.Error("expected purge=false by default")
	}
}

func TestParseUninstallArgs_PurgeOnly(t *testing.T) {
	target, purge := parseUninstallArgs([]string{"--purge"})
	if target != "" {
		t.Errorf("expected no target, got %q", target)
	}
	if !purge {
		t.Error("expected purge=true")
	}
}

func TestParseUninstallArgs_TargetAndPurgeAnyOrder(t *testing.T) {
	target, purge := parseUninstallArgs([]string{"--purge", "cursor"})
	if target != "cursor" {
		t.Errorf("expected target 'cursor', got %q", target)
	}
	if !purge {
		t.Error("expected purge=true")
	}
}

func TestParseUninstallArgs_NoArgs(t *testing.T) {
	target, purge := parseUninstallArgs(nil)
	if target != "" || purge {
		t.Errorf("expected empty target and purge=false, got target=%q purge=%v", target, purge)
	}
}

func TestParseServeFlags_Defaults(t *testing.T) {
	f := parseServeFlags(nil)
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

func TestParseServeFlags_HttpAndPort(t *testing.T) {
	f := parseServeFlags([]string{"--http", "--port", "8080"})
	if !f.http {
		t.Error("expected http=true")
	}
	if f.port != 8080 {
		t.Errorf("expected port 8080, got %d", f.port)
	}
}

func TestParseServeFlags_InvalidPortKeepsDefault(t *testing.T) {
	f := parseServeFlags([]string{"--port", "not-a-number"})
	if f.port != 3000 {
		t.Errorf("expected invalid port to keep default 3000, got %d", f.port)
	}
}

func TestParseServeFlags_PortOutOfRangeKeepsDefault(t *testing.T) {
	f := parseServeFlags([]string{"--port", "70000"})
	if f.port != 3000 {
		t.Errorf("expected out-of-range port to keep default 3000, got %d", f.port)
	}
}

func TestParseServeFlags_RepeatableAllowedHost(t *testing.T) {
	f := parseServeFlags([]string{"--allowed-host", "a.example.com", "--allowed-host", "b.example.com"})
	if len(f.allowedHosts) != 2 || f.allowedHosts[0] != "a.example.com" || f.allowedHosts[1] != "b.example.com" {
		t.Errorf("expected 2 allowed hosts appended in order, got %v", f.allowedHosts)
	}
}

func TestParseServeFlags_HostAndToken(t *testing.T) {
	f := parseServeFlags([]string{"--host", "0.0.0.0", "--token", "secret123"})
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

func TestParseServeFlags_TokenNotPassed_TokenSetFalse(t *testing.T) {
	f := parseServeFlags([]string{"--http"})
	if f.tokenSet {
		t.Error("expected tokenSet=false when --token is not passed at all")
	}
	if f.token != "" {
		t.Errorf("expected empty token, got %q", f.token)
	}
}

func TestParseServeFlags_TokenExplicitEmpty_TokenSetTrue(t *testing.T) {
	// A shell interpolating an empty variable into `--token "$TOKEN"`, or an explicit `--token ""`,
	// must still be distinguishable from "--token wasn't passed at all" — parseServeFlags
	// itself must record that the flag *was* passed, even though its value is empty; resolveToken is
	// what turns that into an error.
	f := parseServeFlags([]string{"--token", ""})
	if !f.tokenSet {
		t.Error("expected tokenSet=true when --token is passed with an explicit empty value")
	}
	if f.token != "" {
		t.Errorf("expected empty token value, got %q", f.token)
	}
}

// The following tests cover resolveToken: the BUILD82_TOKEN env var fallback, and distinguishing
// "no token configured" from "a token source resolved to empty".

func TestResolveToken_NothingConfigured_ReturnsEmptyNoError(t *testing.T) {
	// Neither --token nor BUILD82_TOKEN set at all — the only legitimate way to run without auth.
	token, err := resolveToken(serveFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

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

// TestResolveToken_ExplicitEmptyCliToken_IsConfigError verifies that `--token ""`
// (whether typed literally or produced by shell interpolation of an empty variable) must be a hard
// configuration error, not a silent "auth disabled".
func TestResolveToken_ExplicitEmptyCliToken_IsConfigError(t *testing.T) {
	_, err := resolveToken(serveFlags{token: "", tokenSet: true})
	if err == nil {
		t.Fatal("expected an error for an explicit empty --token")
	}
}

// TestResolveToken_ExplicitEmptyEnvToken_IsConfigError mirrors the above for BUILD82_TOKEN set to an
// empty string (as opposed to left unset entirely).
func TestResolveToken_ExplicitEmptyEnvToken_IsConfigError(t *testing.T) {
	t.Setenv(buildTokenEnvVar, "")
	_, err := resolveToken(serveFlags{})
	if err == nil {
		t.Fatal("expected an error for an explicit empty BUILD82_TOKEN")
	}
}
