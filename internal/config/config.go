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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oito2/mcp-build82/internal/fsutil"
)

// ConfigSource identifies where a resolved Config came from — diagnostic only, not used for
// branching logic elsewhere. Typed the same way every other closed-set value in this codebase is
// (Format, BatchMode, WatchAction, ...) rather than left as a bare string.
type ConfigSource string

const (
	SourceEnv  ConfigSource = "env"
	SourceFile ConfigSource = "file"
)

// Config is the resolved build82 configuration: which Moodle installation to operate on.
type Config struct {
	MoodlePath        string
	MoodleVersion     string
	MoodleFullVersion string
	Source            ConfigSource
}

const configFilename = ".build82"

// configPath resolves ~/.build82. It returns the error from os.UserHomeDir (e.g. an unset
// $HOME/%USERPROFILE%) instead of falling back to a path relative to the working directory.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, configFilename), nil
}

// Load resolves configuration, env wins over file. Returns (nil, nil) if neither source is
// present. Returns a non-nil error only for a genuine failure resolving the config file's location
// (see configPath) — never for "no config file exists yet", which loadFromFile still reports as
// (nil, nil).
func Load() (*Config, error) {
	if c := loadFromEnv(); c != nil {
		return c, nil
	}
	return loadFromFile()
}

func loadFromEnv() *Config {
	path := strings.TrimSpace(os.Getenv("BUILD82_MOODLE_PATH"))
	if path == "" {
		return nil
	}
	return &Config{
		MoodlePath:        path,
		MoodleVersion:     strings.TrimSpace(os.Getenv("BUILD82_MOODLE_VERSION")),
		MoodleFullVersion: strings.TrimSpace(os.Getenv("BUILD82_MOODLE_FULLVERSION")),
		Source:            SourceEnv,
	}
}

func loadFromFile() (*Config, error) {
	cfgPath, err := configPath()
	if err != nil {
		return nil, err
	}
	// Any read error (permission denied, etc.) degrades to "no config from file". A genuine read
	// failure (as opposed to the file not existing, which ReadOptional reports as ok==false,
	// err==nil) is reported with a stderr warning and Load continues with (nil, nil).
	content, ok, err := fsutil.ReadOptional(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build82: warning: could not read config file %s: %v\n", cfgPath, err)
	}
	if !ok {
		return nil, nil
	}
	parsed := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, "=") || strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := ""
		if len(parts) == 2 {
			val = strings.TrimSpace(parts[1])
		}
		parsed[key] = val
	}
	path := parsed["MOODLE_PATH"] // unprefixed key inside the file
	if path == "" {
		return nil, nil
	}
	return &Config{
		MoodlePath:        path,
		MoodleVersion:     parsed["MOODLE_VERSION"],
		MoodleFullVersion: parsed["MOODLE_FULLVERSION"],
		Source:            SourceFile,
	}, nil
}

// Save writes cfg to ~/.build82 as three "KEY=VALUE\n" lines (unprefixed keys). Callers should only
// call Save with a freshly-built Config from tool input — this does not guard against overwriting
// an env-sourced config (deliberately permissive).
//
// There is no cross-process lock on ~/.build82: concurrent Save calls from separate processes race
// and the last writer wins. fsutil.WriteAtomic guarantees the file is never left truncated.
func Save(cfg Config) error {
	// Load() parses this file as "last write wins" per KEY=VALUE line, so a value containing a
	// newline would inject extra lines and let one field's value overwrite another on the next
	// Load(). Such values are rejected.
	for _, v := range []string{cfg.MoodlePath, cfg.MoodleVersion, cfg.MoodleFullVersion} {
		if strings.ContainsAny(v, "\n\r") {
			return fmt.Errorf("config value contains a newline, refusing to write: %q", v)
		}
	}

	cfgPath, err := configPath()
	if err != nil {
		return err
	}

	content := fmt.Sprintf(
		"MOODLE_PATH=%s\nMOODLE_VERSION=%s\nMOODLE_FULLVERSION=%s\n",
		cfg.MoodlePath, cfg.MoodleVersion, cfg.MoodleFullVersion,
	)
	return fsutil.WriteAtomic(cfgPath, []byte(content), 0o644)
}

// Exists reports whether a config is resolvable from either the env var or the config file. The
// second return is non-nil only if the config-file path itself couldn't be resolved (see
// configPath) — that failure must not be conflated with the ordinary "false, no config file
// present" case.
func Exists() (bool, error) {
	if os.Getenv("BUILD82_MOODLE_PATH") != "" {
		return true, nil
	}
	cfgPath, err := configPath()
	if err != nil {
		return false, err
	}
	_, statErr := os.Stat(cfgPath)
	return statErr == nil, nil
}

// FilePath returns the resolved path to the config file, for diagnostic messages (e.g. doctor).
func FilePath() (string, error) {
	return configPath()
}
