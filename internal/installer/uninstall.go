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

package installer

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/generators"
)

// hasEntry reports whether a target currently has a build82 registration configured — not
// merely whether the tool itself is installed.
func hasEntry(t target) bool {
	if t.Shape == shapeCLI {
		_, err := runCommand(t.CLI.Bin, t.CLI.GetArgs...)
		return err == nil
	}
	for _, path := range t.removePaths() {
		if fileHasEntry(path, t.Shape) {
			return true
		}
	}
	return false
}

// fileHasEntry reports whether the config file at `path` is readable and holds a build82 entry
// under the top-level key of `shape`.
func fileHasEntry(path string, shape configShape) bool {
	if !fileExists(path) {
		return false
	}
	m, _, err := readConfig(path)
	if err != nil {
		return false
	}
	sub, ok := m[shape.topKey()].(map[string]any)
	if !ok {
		return false
	}
	_, ok = sub["build82"]
	return ok
}

// removeEntryFile deletes the build82 key from one config file, never touching the rest of the
// file and never creating a file that didn't exist. removed reports whether there was an entry to
// delete. A file with comments or trailing commas is never rewritten.
func removeEntryFile(path string, shape configShape) (removed bool, err error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	m, strict, err := readConfig(path)
	if err != nil {
		return false, err
	}
	topKey := shape.topKey()
	sub, ok := m[topKey].(map[string]any)
	if !ok {
		return false, nil
	}
	if _, ok := sub["build82"]; !ok {
		return false, nil
	}
	if !strict {
		return false, fmt.Errorf("%s contains comments or trailing commas; it was left unchanged so they are not lost. "+
			"Remove the \"build82\" entry from its %q object manually", path, topKey)
	}
	delete(sub, "build82")
	m[topKey] = sub
	return true, writeJSON(path, m)
}

// uninstallTarget removes build82 from a target. removed is false, with a nil error, when there
// was no registration to remove. warnings describe registrations deliberately left in place.
func uninstallTarget(t target) (removed bool, warnings []string, err error) {
	if t.Shape == shapeCLI {
		return removeCLIRegistration(t.CLI)
	}
	var errs []error
	for _, path := range t.removePaths() {
		ok, err := removeEntryFile(path, t.Shape)
		if err != nil {
			errs = append(errs, err)
		}
		removed = removed || ok
	}
	return removed, nil, errors.Join(errs...)
}

// reportUninstall prints the outcome of one target's uninstall.
func reportUninstall(t target, removed bool, warnings []string, err error) {
	switch {
	case err != nil:
		fmt.Printf("%s... failed: %v\n", t.Label, err)
	case removed:
		fmt.Printf("%s... removed.\n", t.Label)
	case len(warnings) > 0:
		fmt.Printf("%s... nothing removed.\n", t.Label)
	default:
		fmt.Printf("%s... not registered.\n", t.Label)
	}
	printWarnings(warnings)
}

// purgePluginFilenames is every filename a --purge pass considers per development plugin: the
// generated context files plus the .indevelopment marker. The mtime cache file (.cache.json) is
// not included.
var purgePluginFilenames = append(append([]string{}, generators.PluginContextFiles...), ".indevelopment")

// purgeCandidates returns the config file path (which may not exist) and every generated file
// that currently exists and would be deleted by --purge: the global context files of the Moodle
// root in `cfg` plus those of its .indevelopment-marked dev plugins. A nil `cfg` yields no files.
// The error is non-nil only if the config file's location cannot be resolved.
func purgeCandidates(cfg *config.Config) (configFile string, files []string, err error) {
	configFile, err = config.FilePath()
	if err != nil {
		return "", nil, err
	}
	if cfg == nil {
		return configFile, nil, nil
	}
	for _, f := range generators.GlobalContextFilenames {
		p := generators.GlobalOutputPath(cfg.MoodlePath, f)
		if fileExists(p) {
			files = append(files, p)
		}
	}
	for _, dir := range generators.FindDevPlugins(cfg.MoodlePath) {
		for _, f := range purgePluginFilenames {
			p := generators.PluginOutputPath(dir, f)
			if fileExists(p) {
				files = append(files, p)
			}
		}
	}
	return configFile, files, nil
}

// runPurge implements the --purge flag: it lists what will be deleted, asks a separate
// confirmation read from `in`, then removes the ~/.build82 config file and the generated files of
// `cfg`'s Moodle root. Individual removal failures are printed, not returned; the error is non-nil
// only when the config location cannot be resolved.
func runPurge(cfg *config.Config, in *bufio.Reader) error {
	configFile, files, err := purgeCandidates(cfg)
	if err != nil {
		return err
	}

	fmt.Println("\n--purge will delete:")
	if fileExists(configFile) {
		fmt.Printf("  - config file: %s\n", configFile)
	}
	fmt.Printf("  - %d generated file(s) under %s/\n", len(files), generators.ContextDir)

	if !confirm(in, "This cannot be undone. Proceed? [y/N] ") {
		fmt.Println("Purge cancelled.")
		return nil
	}

	if fileExists(configFile) {
		if err := os.Remove(configFile); err != nil {
			fmt.Printf("failed to remove %s: %v\n", configFile, err)
		}
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			fmt.Printf("failed to remove %s: %v\n", f, err)
		}
	}
	fmt.Println("Purge complete.")
	return nil
}

// Uninstall is the top-level `build82 uninstall [target] [--purge]` flow. With a non-empty
// `targetID` it removes build82 from that target only; otherwise it removes it from every tool that
// has a registration, after confirmation. When `purge` is true it then runs the purge step. It
// returns an error for an unknown target, a failed single-target removal, or a config load failure.
func Uninstall(targetID string, purge bool) error {
	in := bufio.NewReader(os.Stdin)

	if targetID != "" {
		t, ok, err := targetByID(targetID)
		if err != nil {
			return err
		}
		if !ok {
			ids, err := supportedIDs()
			if err != nil {
				return err
			}
			return fmt.Errorf("unknown target %q — supported targets: %s", targetID, strings.Join(ids, ", "))
		}
		switch {
		case t.Unsupported != "":
			fmt.Printf("Skipped: %s (%s).\n", t.Label, t.Unsupported)
		case t.Shape == shapeCLI && !detectTarget(t):
			fmt.Printf("Skipped: %s not detected.\n", t.Label)
		default:
			removed, warnings, err := uninstallTarget(t)
			reportUninstall(t, removed, warnings, err)
			if err != nil {
				return err
			}
		}
	} else {
		ts, err := targets()
		if err != nil {
			return err
		}
		var configured []target
		for _, t := range ts {
			if hasEntry(t) {
				configured = append(configured, t)
			}
		}
		if len(configured) == 0 {
			fmt.Println("No build82 registrations found.")
		} else {
			fmt.Println("Found build82 registered in the following tool(s):")
			for _, t := range configured {
				fmt.Printf("  - %s\n", t.Label)
			}
			if !confirm(in, fmt.Sprintf("Remove build82 from all %d tool(s)? [y/N] ", len(configured))) {
				return nil
			}
			for _, t := range configured {
				removed, warnings, err := uninstallTarget(t)
				reportUninstall(t, removed, warnings, err)
			}
		}
	}

	if !purge {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return runPurge(cfg, in)
}
