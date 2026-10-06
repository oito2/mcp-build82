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

package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHome points os.UserHomeDir() (and therefore configPath()) at a fresh temp directory, so
// these tests never touch the real user's actual ~/.build82 file.
func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows equivalent, harmless on other OSes
	return dir
}

// clearEnv empties the BUILD82_MOODLE_* environment variables for the duration of the test.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BUILD82_MOODLE_PATH", "")
	t.Setenv("BUILD82_MOODLE_VERSION", "")
	t.Setenv("BUILD82_MOODLE_FULLVERSION", "")
}

// TestLoadConfig_FromEnv verifies that Load reads the environment variables and trims their values.
func TestLoadConfig_FromEnv(t *testing.T) {
	withHome(t)
	t.Setenv("BUILD82_MOODLE_PATH", "/tmp/test-moodle")
	t.Setenv("BUILD82_MOODLE_VERSION", "4.4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config from env, got nil")
	}
	if cfg.MoodlePath != "/tmp/test-moodle" || cfg.MoodleVersion != "4.4" || cfg.Source != "env" {
		t.Errorf("config mismatch: %+v", cfg)
	}
}

// TestLoadConfig_FromFile verifies that Load reads ~/.build82 when the environment is empty.
func TestLoadConfig_FromFile(t *testing.T) {
	home := withHome(t)
	clearEnv(t)

	content := "MOODLE_PATH=/var/www/moodle\nMOODLE_VERSION=4.3\nMOODLE_FULLVERSION=2023110900\n"
	if err := os.WriteFile(filepath.Join(home, ".build82"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config from file, got nil")
	}
	if cfg.MoodlePath != "/var/www/moodle" || cfg.MoodleVersion != "4.3" || cfg.MoodleFullVersion != "2023110900" || cfg.Source != "file" {
		t.Errorf("config mismatch: %+v", cfg)
	}
}

// TestLoadConfig_EnvWinsOverFile verifies that the environment takes precedence over the file.
func TestLoadConfig_EnvWinsOverFile(t *testing.T) {
	home := withHome(t)
	content := "MOODLE_PATH=/from/file\n"
	if err := os.WriteFile(filepath.Join(home, ".build82"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUILD82_MOODLE_PATH", "/from/env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || cfg.MoodlePath != "/from/env" || cfg.Source != "env" {
		t.Errorf("expected env to win over file, got %+v", cfg)
	}
}

// TestLoadConfig_MalformedLinesAndCommentsIgnored verifies that comment lines and lines without "=" are skipped.
func TestLoadConfig_MalformedLinesAndCommentsIgnored(t *testing.T) {
	home := withHome(t)
	clearEnv(t)

	content := "# a comment line\nnotakeyvaluepair\nMOODLE_PATH=/var/www/moodle\n  # indented comment\nMOODLE_VERSION=4.3\n"
	if err := os.WriteFile(filepath.Join(home, ".build82"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || cfg.MoodlePath != "/var/www/moodle" || cfg.MoodleVersion != "4.3" {
		t.Errorf("expected malformed/comment lines to be skipped, got %+v", cfg)
	}
}

// TestLoadConfig_NoConfigAtAll verifies that Load returns (nil, nil) when no config exists.
func TestLoadConfig_NoConfigAtAll(t *testing.T) {
	withHome(t)
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil with no env and no file, got %+v", cfg)
	}
}

// TestLoadConfig_FromFile_HomeDirUnresolvableReturnsError verifies that Load (env unset, so it
// falls through to the file path) returns a real error when the home directory cannot be resolved,
// instead of degrading to a cwd-relative path or to "no config found".
func TestLoadConfig_FromFile_HomeDirUnresolvableReturnsError(t *testing.T) {
	clearEnv(t)
	// os.UserHomeDir() on Unix returns an error specifically when $HOME is set-but-empty (as
	// opposed to unset, which can still fall back to os/user on some platforms) — t.Setenv leaves
	// the variable defined for the duration of the test either way.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved, got nil")
	}
	if cfg != nil {
		t.Errorf("expected a nil config alongside the error, got %+v", cfg)
	}
}

// TestLoadConfig_ReadErrorWarnsOnStderrButDegradesGracefully verifies that a genuine read failure
// on the config file (as opposed to it not existing) makes loadFromFile (via Load) print a warning
// to stderr, while still returning (nil, nil) rather than a hard error.
//
// A directory is created at ~/.build82 (instead of a regular file) to force os.ReadFile to fail with
// a real error distinct from "not exist" (errors.Is(err, os.ErrNotExist) is false for "is a
// directory"), without relying on permission bits that behave inconsistently when tests run as root.
func TestLoadConfig_ReadErrorWarnsOnStderrButDegradesGracefully(t *testing.T) {
	home := withHome(t)
	clearEnv(t)

	if err := os.MkdirAll(filepath.Join(home, ".build82"), 0o755); err != nil {
		t.Fatal(err)
	}

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	origStderr := os.Stderr
	os.Stderr = w

	cfg, err := Load()

	os.Stderr = origStderr
	if closeErr := w.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatal(copyErr)
	}

	if err != nil {
		t.Fatalf("expected Load to degrade gracefully (no fatal error) on a real read failure, got: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected a nil config when the config file can't be read, got %+v", cfg)
	}

	captured := buf.String()
	if !strings.Contains(captured, "could not read config file") {
		t.Errorf("expected a stderr warning about the unreadable config file, got: %q", captured)
	}
}

// TestSave_WritesExpectedFormat verifies the file content written by Save and that Load reads it back.
func TestSave_WritesExpectedFormat(t *testing.T) {
	home := withHome(t)

	err := Save(Config{MoodlePath: "/var/www/moodle", MoodleVersion: "4.3", MoodleFullVersion: "2023110900"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(home, ".build82"))
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	want := "MOODLE_PATH=/var/www/moodle\nMOODLE_VERSION=4.3\nMOODLE_FULLVERSION=2023110900\n"
	if string(content) != want {
		t.Errorf("got %q, want %q", content, want)
	}

	// Round-trip: Save then Load (via file, since env is untouched here) should agree.
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || cfg.MoodlePath != "/var/www/moodle" {
		t.Errorf("round-trip mismatch: %+v", cfg)
	}
}

// TestSave_RejectsNewlineInValue verifies that Save rejects a value containing "\n", which Load
// would otherwise read back as extra KEY=VALUE lines ("last write wins" per key).
func TestSave_RejectsNewlineInValue(t *testing.T) {
	home := withHome(t)

	err := Save(Config{MoodlePath: "/var/www/moodle\nMOODLE_VERSION=9.9.9", MoodleVersion: "4.3", MoodleFullVersion: "2023110900"})
	if err == nil {
		t.Fatal("expected an error for a MoodlePath containing a newline")
	}

	if _, statErr := os.Stat(filepath.Join(home, ".build82")); statErr == nil {
		t.Error("expected no config file to be written when a value contains a newline")
	}
}

// TestSave_RejectsCarriageReturnInValue verifies that Save rejects "\r" as well as "\n", since
// either alone injects a line break when the file is read back line by line.
func TestSave_RejectsCarriageReturnInValue(t *testing.T) {
	withHome(t)

	err := Save(Config{MoodlePath: "/var/www/moodle", MoodleVersion: "4.3\rMOODLE_FULLVERSION=evil", MoodleFullVersion: "2023110900"})
	if err == nil {
		t.Fatal("expected an error for a MoodleVersion containing a carriage return")
	}
}

// TestSave_HomeDirUnresolvableReturnsError is configPath's Save-side counterpart: Save must not
// write to a cwd-relative fallback path when the home directory can't be resolved.
func TestSave_HomeDirUnresolvableReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	err := Save(Config{MoodlePath: "/var/www/moodle"})
	if err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
}

// TestExists verifies Exists for the environment variable, the file, and neither.
func TestExists(t *testing.T) {
	home := withHome(t)
	clearEnv(t)

	if exists, err := Exists(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if exists {
		t.Error("expected Exists()=false with no env and no file")
	}

	t.Setenv("BUILD82_MOODLE_PATH", "/tmp/x")
	if exists, err := Exists(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if !exists {
		t.Error("expected Exists()=true when the env var is set")
	}
	clearEnv(t)

	if err := os.WriteFile(filepath.Join(home, ".build82"), []byte("MOODLE_PATH=/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if exists, err := Exists(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if !exists {
		t.Error("expected Exists()=true when the config file is present")
	}
}

// TestExists_HomeDirUnresolvableReturnsError is configPath's Exists-side counterpart, distinguishing
// a genuine resolution failure from the ordinary "no config file present" (false, nil) case.
func TestExists_HomeDirUnresolvableReturnsError(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	exists, err := Exists()
	if err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
	if exists {
		t.Error("expected exists=false alongside the error")
	}
}

// TestFilePath verifies that FilePath returns ~/.build82.
func TestFilePath(t *testing.T) {
	home := withHome(t)
	got, err := FilePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(home, ".build82"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFilePath_HomeDirUnresolvableReturnsError is configPath's FilePath-side counterpart.
func TestFilePath_HomeDirUnresolvableReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	if _, err := FilePath(); err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
}

// TestExists_BlankEnvVarIsNotSet verifies that a whitespace-only BUILD82_MOODLE_PATH does not
// count as configured, matching how Load treats it.
func TestExists_BlankEnvVarIsNotSet(t *testing.T) {
	withHome(t)
	clearEnv(t)
	t.Setenv("BUILD82_MOODLE_PATH", "   ")

	if exists, err := Exists(); err != nil || exists {
		t.Errorf("Exists() = %v, %v; want false, nil", exists, err)
	}
}
