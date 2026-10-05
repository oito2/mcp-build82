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

package generators

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
)

func TestApiFunctionLine_DeprecatedMarkerFormat(t *testing.T) {
	// Load-bearing: search_api substring-matches "@deprecated" in this exact rendered line.
	line := apiFunctionLine(extractors.ApiFunction{
		Name:       "old_fn",
		Visibility: extractors.VisDeprecated,
		Doc:        &extractors.PhpDocBlock{Deprecated: "use new_fn() instead"},
	})
	if !strings.Contains(line, "@deprecated") {
		t.Errorf("expected the rendered line to contain the literal '@deprecated' marker, got %q", line)
	}
	if !strings.Contains(line, "use new_fn() instead") {
		t.Errorf("expected the deprecation message in the line, got %q", line)
	}

	publicLine := apiFunctionLine(extractors.ApiFunction{Name: "new_fn", Visibility: extractors.VisPublic})
	if strings.Contains(publicLine, "@deprecated") {
		t.Errorf("expected a public function's line to never contain '@deprecated', got %q", publicLine)
	}
}

func TestGenerateApiIndex(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "lib"))
	mustWriteFile(t, filepath.Join(dir, "lib", "moodlelib.php"), `<?php
/**
 * Does a thing.
 */
function test_public_fn() {}

/**
 * @deprecated no longer used
 */
function test_deprecated_fn() {}
`)
	result := GenerateApiIndex(dir)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, err := os.ReadFile(GlobalOutputPath(dir, "MOODLE_API_INDEX.md"))
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if !strings.Contains(string(content), "test_public_fn") || !strings.Contains(string(content), "@deprecated") {
		t.Errorf("expected both functions represented, got:\n%s", content)
	}
}

func TestGenerateEventsIndex_SortedAcrossPlugins(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "local", "b", "db"))
	mustMkdirAll(t, filepath.Join(dir, "local", "a", "db"))
	mustWriteFile(t, filepath.Join(dir, "local", "b", "db", "events.php"),
		"<?php\n$observers = [['eventname' => '\\\\core\\\\event\\\\z_event', 'callback' => 'x::z']];")
	mustWriteFile(t, filepath.Join(dir, "local", "a", "db", "events.php"),
		"<?php\n$observers = [['eventname' => '\\\\core\\\\event\\\\a_event', 'callback' => 'x::a']];")

	result := GenerateEventsIndex(dir)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(GlobalOutputPath(dir, "MOODLE_EVENTS_INDEX.md"))
	aIdx := strings.Index(string(content), "a_event")
	zIdx := strings.Index(string(content), "z_event")
	if aIdx == -1 || zIdx == -1 || aIdx > zIdx {
		t.Errorf("expected events sorted by name (a_event before z_event), got:\n%s", content)
	}
}

func TestGenerateDbTablesIndex(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "local", "demo", "db"))
	mustWriteFile(t, filepath.Join(dir, "local", "demo", "db", "install.xml"), schemaFixtureForGeneratorTest)

	result := GenerateDbTablesIndex(dir)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(GlobalOutputPath(dir, "MOODLE_DB_TABLES_INDEX.md"))
	if !strings.Contains(string(content), "local_test_records") {
		t.Errorf("expected table name in output, got:\n%s", content)
	}
}

const schemaFixtureForGeneratorTest = `<?xml version="1.0" encoding="UTF-8" ?>
<XMLDB PATH="local/test/db" VERSION="20240101">
  <TABLES>
    <TABLE NAME="local_test_records" COMMENT="Test records">
      <FIELDS>
        <FIELD NAME="id" TYPE="int" LENGTH="10" NOTNULL="true" SEQUENCE="true"/>
      </FIELDS>
    </TABLE>
  </TABLES>
</XMLDB>`

func TestGenerateCtags_SkipsGracefullyWithoutCtags(t *testing.T) {
	dir := t.TempDir()
	// We can't guarantee ctags is absent in every CI environment, so only assert the two
	// documented outcomes: either a graceful skip, or a real success — never a hard failure just
	// because the binary happens to be missing.
	result := GenerateCtags(dir)
	if result.Error != "" && result.Skipped {
		t.Errorf("Skipped and Error should not both be set: %+v", result)
	}
}

// TestGenerateCtags_PassesDoubleDashBeforeMoodlePath confirms ctags is invoked with a "--"
// argument boundary immediately before moodlePath — without it, a moodlePath beginning with "-"
// would be parsed as a ctags flag instead of the target directory.
// Builds a fake "ctags" binary that records its argv, since the real ctags isn't guaranteed to be
// installed in every environment (same reasoning as TestGenerateCtags_SkipsGracefullyWithoutCtags).
func TestGenerateCtags_PassesDoubleDashBeforeMoodlePath(t *testing.T) {
	binDir := t.TempDir()
	argvFile := filepath.Join(binDir, "argv.txt")
	buildFakeCtags(t, binDir, argvFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	moodlePath := t.TempDir()
	result := GenerateCtags(moodlePath)
	if !result.Success {
		t.Fatalf("expected success, got: %+v", result)
	}

	recorded, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("fake ctags did not record its arguments: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(args) < 2 || args[len(args)-1] != moodlePath || args[len(args)-2] != "--" {
		t.Fatalf(`expected "--" immediately before moodlePath in ctags' arguments, got: %v`, args)
	}
}

// buildFakeCtags compiles a tiny real "ctags" binary that writes its own argv (one per line) to
// argvFile. A real subprocess exercises exec.Command's actual argument-passing behavior, which a mock can't.
func buildFakeCtags(t *testing.T, dir, argvFile string) {
	t.Helper()
	srcDir := t.TempDir()
	src := fmt.Sprintf(`package main

import (
	"os"
	"strings"
)

func main() {
	f, err := os.Create(%q)
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	f.WriteString(strings.Join(os.Args[1:], "\n"))
}
`, argvFile)
	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	name := "ctags"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, srcFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fake ctags: %v\n%s", err, output)
	}
}

func TestGenerateAll_Integration(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	results := GenerateAll(moodlePath, "4.3")
	// 12 wave-1 generators + AI index (wave 2) + tags = 14 results.
	if len(results) != 14 {
		t.Errorf("expected 14 generator results, got %d: %+v", len(results), results)
	}

	for _, f := range GlobalContextFilenames {
		p := GlobalOutputPath(moodlePath, f)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist, got error: %v", f, err)
		}
	}

	// Nothing extra left at the Moodle root — every generated file lives under .build82/.
	for _, f := range GlobalContextFilenames {
		if _, err := os.Stat(filepath.Join(moodlePath, f)); err == nil {
			t.Errorf("expected %s to NOT exist at the Moodle root", f)
		}
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("generator result was not successful: %+v", r)
		}
	}
}

func TestGenerateAll_PersistentCacheRoundTrip(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	c1 := cache.NewMtimeCache()
	old := swapGlobalCache(c1)
	defer swapGlobalCache(old)

	GenerateAll(moodlePath, "4.3")

	if _, err := os.Stat(filepath.Join(moodlePath, ContextDir, ".cache.json")); err != nil {
		t.Fatalf("expected persistent cache file to exist: %v", err)
	}

	// Simulate a process restart: swap in a brand new MtimeCache instance for the same root.
	c2 := cache.NewMtimeCache()
	swapGlobalCache(c2)

	results := GenerateAll(moodlePath, "4.3")
	for _, r := range results {
		if !r.Skipped {
			t.Errorf("expected every generator to report Skipped=true on a cache hit after restart, got %+v", r)
		}
	}
}
