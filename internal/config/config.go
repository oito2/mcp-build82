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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oito2/mcp-build82/internal/fsutil"
)

// ConfigSource identifies where a resolved Config came from. It is informational only.
type ConfigSource string

// Values of ConfigSource.
const (
	SourceEnv  ConfigSource = "env"
	SourceFile ConfigSource = "file"
)

// Config is the resolved build82 configuration: which Moodle installation to operate on.
type Config struct {
	// MoodlePath is the Moodle root directory.
	MoodlePath string
	// MoodleVersion is the Moodle release version string; it may be empty.
	MoodleVersion string
	// MoodleFullVersion is the full Moodle version identifier; it may be empty.
	MoodleFullVersion string
	// Source records whether the values came from the environment or the config file.
	Source ConfigSource
}

// configFilename is the name of the config file inside the home directory.
const configFilename = ".build82"

// configPath returns the path of ~/.build82. It returns the os.UserHomeDir error (for example an
// unset $HOME/%USERPROFILE%) rather than falling back to a relative path.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, configFilename), nil
}

// Load resolves the configuration; the BUILD82_MOODLE_* environment variables take precedence
// over ~/.build82. It returns (nil, nil) when neither source provides a Moodle path, and an error
// only when the config file's location cannot be resolved.
func Load() (*Config, error) {
	if c := loadFromEnv(); c != nil {
		return c, nil
	}
	return loadFromFile()
}

// loadFromEnv builds a Config from the BUILD82_MOODLE_* environment variables, or returns nil when
// BUILD82_MOODLE_PATH is empty. Values are trimmed of surrounding whitespace.
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

// loadFromFile builds a Config from the KEY=VALUE lines of ~/.build82. It returns (nil, nil) when
// the file is missing, unreadable or has no MOODLE_PATH; an unreadable file also prints a warning to
// stderr. The error is non-nil only when the file's location cannot be resolved.
func loadFromFile() (*Config, error) {
	cfgPath, err := configPath()
	if err != nil {
		return nil, err
	}
	// A read failure other than a missing file only warns and is treated as no config.
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

// Save writes `cfg` to ~/.build82 as three "KEY=VALUE\n" lines (MOODLE_PATH, MOODLE_VERSION,
// MOODLE_FULLVERSION). It returns an error when a value contains a newline or the file cannot be
// written. Source is not persisted, and an env-sourced Config is written like any other.
//
// There is no cross-process lock: concurrent Save calls race and the last writer wins, but the
// write is atomic so the file is never left truncated.
func Save(cfg Config) error {
	// A newline in a value would inject extra KEY=VALUE lines that override other fields on load.
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

// Exists reports whether BUILD82_MOODLE_PATH is set to a non-blank value or the config file exists. The error is non-nil
// only when the config file's location cannot be resolved. The file's content is not validated.
func Exists() (bool, error) {
	if strings.TrimSpace(os.Getenv("BUILD82_MOODLE_PATH")) != "" {
		return true, nil
	}
	cfgPath, err := configPath()
	if err != nil {
		return false, err
	}
	_, statErr := os.Stat(cfgPath)
	return statErr == nil, nil
}

// FilePath returns the path of the config file (~/.build82), or an error when the home directory
// cannot be resolved.
func FilePath() (string, error) {
	return configPath()
}
