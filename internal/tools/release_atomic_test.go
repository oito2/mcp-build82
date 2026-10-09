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

package tools

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// assertOnlyEntries fails unless dir contains exactly the named entries, so a failed
// createZip can be checked to leave neither a partial archive nor its temporary file behind.
func assertOnlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("entries in output dir = %v, want %v", got, want)
	}
}

// TestCreateZip_WalkFailureLeavesNoPartialArchive makes one plugin file unreadable so the walk
// fails after other entries were already written: nothing may remain at the destination.
func TestCreateZip_WalkFailureLeavesNoPartialArchive(t *testing.T) {
	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, "a.php"), "<?php\n")
	unreadable := filepath.Join(pluginPath, "z.php")
	mustWriteFile(t, unreadable, "<?php\n")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
	if f, err := os.Open(unreadable); err == nil {
		f.Close()
		t.Skip("file permissions are not enforced for this user")
	}

	outDir := t.TempDir()
	if _, _, err := createZip(pluginPath, filepath.Join(outDir, "out.zip"), "myplugin", excludedNames); err == nil {
		t.Fatal("expected createZip to fail on an unreadable file")
	}
	assertOnlyEntries(t, outDir)
}

// TestCreateZip_CloseFailureKeepsPreviousArchive confirms a failure while flushing the archive
// neither truncates nor replaces an archive already present at the destination.
func TestCreateZip_CloseFailureKeepsPreviousArchive(t *testing.T) {
	pluginPath := t.TempDir()
	outDir := t.TempDir()
	dest := filepath.Join(outDir, "out.zip")
	mustWriteFile(t, dest, "previous archive")

	orig := newZipDestination
	t.Cleanup(func() { newZipDestination = orig })
	newZipDestination = func(path string) (io.WriteCloser, error) {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		return writeFailCloser{File: f}, nil
	}

	if _, _, err := createZip(pluginPath, dest, "myplugin", excludedNames); err == nil {
		t.Fatal("expected createZip to fail")
	}
	if got := mustReadFile(t, dest); got != "previous archive" {
		t.Errorf("existing archive was modified: %q", got)
	}
	assertOnlyEntries(t, outDir, "out.zip")
}

// TestCreateZip_SuccessLeavesOnlyFinalArchive confirms the temporary file is renamed away on
// success, and that an output directory inside the plugin never archives the ZIP into itself.
func TestCreateZip_SuccessLeavesOnlyFinalArchive(t *testing.T) {
	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\n")
	outDir := filepath.Join(pluginPath, "dist")
	mustMkdirAll(t, outDir)
	dest := filepath.Join(outDir, "out.zip")

	if _, _, err := createZip(pluginPath, dest, "myplugin", excludedNames); err != nil {
		t.Fatalf("createZip: %v", err)
	}
	assertOnlyEntries(t, outDir, "out.zip")

	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "myplugin/dist/") {
			t.Errorf("archive contains its own output file: %s", f.Name)
		}
	}
	if info, err := os.Stat(dest); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o644) {
		t.Errorf("expected a 0644 archive, got %v (err %v)", info.Mode().Perm(), err)
	}
}

// TestHandleReleasePlugin_OutputPathsAreNotAbsolute covers both ZIP locations: outside the Moodle
// root the ZIP is reported by file name only, inside it relative to the Moodle root. The source
// directory is always reported relative to the Moodle root. Also exercises a path identifier
// under strict mode, whose expected component is derived from the plugin's location.
func TestHandleReleasePlugin_OutputPathsAreNotAbsolute(t *testing.T) {
	moodlePath, pluginPath := setupResolveFixture(t)
	writeValidReleaseFixture(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n"+
			"$plugin->requires = 2022041900;\n$plugin->maturity = MATURITY_STABLE;\n")
	mustWriteFile(t, filepath.Join(pluginPath, "lang", "en", "local_demo.php"), "<?php\n$string['pluginname'] = 'Demo';\n")

	insideOut := filepath.Join(moodlePath, "releases")
	mustMkdirAll(t, insideOut)

	cases := []struct {
		name, outDir, wantOutput string
	}{
		{"outside moodle", t.TempDir(), "Output: local_demo_2024010100.zip\n"},
		{"inside moodle", insideOut, "Output: releases/local_demo_2024010100.zip\n"},
	}
	for _, c := range cases {
		res, _, err := handleReleasePlugin(context.Background(), nil, ReleasePluginInput{
			Component: "local/demo", OutputDir: c.outDir, Strict: true,
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		text := resolveResultText(t, res)
		if res.IsError {
			t.Fatalf("%s: unexpected error result: %s", c.name, text)
		}
		if !strings.Contains(text, c.wantOutput) || !strings.Contains(text, "Source: local/demo\n") {
			t.Errorf("%s: unexpected paths in response: %s", c.name, text)
		}
		if strings.Contains(text, moodlePath) || strings.Contains(text, c.outDir) {
			t.Errorf("%s: response leaks an absolute path: %s", c.name, text)
		}
		if _, err := os.Stat(filepath.Join(c.outDir, "local_demo_2024010100.zip")); err != nil {
			t.Errorf("%s: ZIP not written: %v", c.name, err)
		}
	}
}
