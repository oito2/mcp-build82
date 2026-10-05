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

package extractors

import (
	"os"
	"path/filepath"
	"testing"
)

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const moodleVersionPhpFixture = `<?php
defined('MOODLE_INTERNAL') || die();

$version  = 2023110900.00;
$release  = '4.3+ (Build: 20231109)';
$branch   = '403';
$maturity = MATURITY_STABLE;
`

func TestDetectMoodleInstall(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), moodleVersionPhpFixture)

	info := DetectMoodleInstall(dir)
	if info == nil {
		t.Fatal("expected non-nil install info")
	}
	if info.Version != "4.3+" {
		t.Errorf("Version = %q, want %q", info.Version, "4.3+")
	}
	if info.Build != "2023110900.00" {
		t.Errorf("Build = %q, want %q", info.Build, "2023110900.00")
	}
	if info.Branch != "403" {
		t.Errorf("Branch = %q, want %q", info.Branch, "403")
	}
	if info.Release != "4.3+ (Build: 20231109)" {
		t.Errorf("Release = %q", info.Release)
	}
}

func TestDetectMoodleInstall_FallsBackToBuildWhenReleaseAbsent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$version = 2023110900.00;\n$branch = '403';\n")

	info := DetectMoodleInstall(dir)
	if info == nil {
		t.Fatal("expected non-nil install info")
	}
	if info.Version != info.Build {
		t.Errorf("expected Version to fall back to Build when release-prefix extraction fails, got Version=%q Build=%q", info.Version, info.Build)
	}
}

func TestDetectMoodleInstall_MissingFile(t *testing.T) {
	if DetectMoodleInstall(t.TempDir()) != nil {
		t.Error("expected nil for a directory with no version.php")
	}
}

func TestIsMoodleRoot(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), moodleVersionPhpFixture)
	mustMkdirAll(t, filepath.Join(dir, "lib"))
	mustWriteFile(t, filepath.Join(dir, "config.php"), "<?php\n")

	if !IsMoodleRoot(dir) {
		t.Error("expected true for a well-formed Moodle root")
	}
}

func TestIsMoodleRoot_ConfigDistFallback(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), moodleVersionPhpFixture)
	mustMkdirAll(t, filepath.Join(dir, "lib"))
	mustWriteFile(t, filepath.Join(dir, "config-dist.php"), "<?php\n")

	if !IsMoodleRoot(dir) {
		t.Error("expected true when only config-dist.php is present")
	}
}

func TestIsMoodleRoot_MissingLibDir(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), moodleVersionPhpFixture)
	mustWriteFile(t, filepath.Join(dir, "config.php"), "<?php\n")

	if IsMoodleRoot(dir) {
		t.Error("expected false without a lib/ directory")
	}
}

func TestIsMoodleRoot_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), moodleVersionPhpFixture)
	mustMkdirAll(t, filepath.Join(dir, "lib"))

	if IsMoodleRoot(dir) {
		t.Error("expected false without config.php or config-dist.php")
	}
}
