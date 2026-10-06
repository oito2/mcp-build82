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

package server

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/generators"
)

// TestE2E_TreesitterBackend_TestdataFixture runs init_moodle_context and generate_plugin_context
// with the tree-sitter extractor backend against the shared testdata/moodle fixture over an
// in-memory MCP session, then checks the content of the generated Markdown files.
func TestE2E_TreesitterBackend_TestdataFixture(t *testing.T) {
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	t.Setenv("BUILD82_MOODLE_PATH", root)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()

	initResult := callTool(t, session, "init_moodle_context", map[string]any{"moodle_path": root})
	if initResult.IsError {
		t.Fatalf("init_moodle_context: %s", toolText(t, initResult))
	}

	genResult := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local/demo"})
	if genResult.IsError {
		t.Fatalf("generate_plugin_context: %s", toolText(t, genResult))
	}

	assertSaneDemoContext(t, root)
}

// assertSaneDemoContext reads the generated PLUGIN_*.md files of local/demo under the Moodle root
// `root` and checks that they contain the values present in the fixture source, which shows that
// the extractor found real data.
func assertSaneDemoContext(t *testing.T, root string) {
	t.Helper()
	pluginDir := filepath.Join(root, "local", "demo")

	read := func(filename string) string {
		t.Helper()
		content, err := os.ReadFile(generators.PluginOutputPath(pluginDir, filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		return string(content)
	}

	contextMd := read("PLUGIN_CONTEXT.md")
	for _, want := range []string{"local_demo", "2024010100", "MATURITY_STABLE"} {
		if !strings.Contains(contextMd, want) {
			t.Errorf("PLUGIN_CONTEXT.md missing %q:\n%s", want, contextMd)
		}
	}

	events := read("PLUGIN_EVENTS.md")
	for _, want := range []string{"course_viewed", "observer::course_viewed"} {
		if !strings.Contains(events, want) {
			t.Errorf("PLUGIN_EVENTS.md missing %q:\n%s", want, events)
		}
	}

	funcIndex := read("PLUGIN_FUNCTION_INDEX.md")
	if !strings.Contains(funcIndex, "local_demo_before_footer") {
		t.Errorf("PLUGIN_FUNCTION_INDEX.md missing local_demo_before_footer:\n%s", funcIndex)
	}

	callbackIndex := read("PLUGIN_CALLBACK_INDEX.md")
	for _, want := range []string{"local_demo_before_footer", "before_http_headers", "data_submitted"} {
		if !strings.Contains(callbackIndex, want) {
			t.Errorf("PLUGIN_CALLBACK_INDEX.md missing %q:\n%s", want, callbackIndex)
		}
	}

	// The two version-gated steps of db/upgrade.php (2023120100, 2024010100) appear in the
	// "Upgrade History" section of PLUGIN_DEPENDENCIES.md.
	dependencies := read("PLUGIN_DEPENDENCIES.md")
	for _, want := range []string{"2023120100", "2024010100"} {
		if !strings.Contains(dependencies, want) {
			t.Errorf("PLUGIN_DEPENDENCIES.md missing upgrade step %q:\n%s", want, dependencies)
		}
	}

	aiContext := read("PLUGIN_AI_CONTEXT.md")
	if len(aiContext) < 200 {
		t.Errorf("PLUGIN_AI_CONTEXT.md suspiciously short (%d bytes):\n%s", len(aiContext), aiContext)
	}
}

// TestE2E_TreesitterBackend_RealPlugins runs the tree-sitter backend on a few plugins (a mod, a
// block and an admin tool) copied from a real Moodle installation at a fixed local path into a
// temporary Moodle root, so the installation is never written to. The test is skipped when that
// installation or the sample plugins are not present.
func TestE2E_TreesitterBackend_RealPlugins(t *testing.T) {
	const realInstall = "/srv/workspace/www/html/mdle/dev-500"
	if _, err := os.Stat(filepath.Join(realInstall, "version.php")); err != nil {
		t.Skipf("real Moodle install not available at %s: %v", realInstall, err)
	}

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t) // supplies the files that make the directory a Moodle root.
	t.Setenv("BUILD82_MOODLE_PATH", root)

	// Plugins of different types: a mod, a block and an admin tool.
	samples := []string{
		"mod/forum",
		"blocks/html",
		"admin/tool/log",
	}

	var copied []string
	for _, rel := range samples {
		src := filepath.Join(realInstall, rel)
		if _, err := os.Stat(filepath.Join(src, "version.php")); err != nil {
			continue // skip samples missing from this installation
		}
		dst := filepath.Join(root, rel)
		copyRealPluginTree(t, src, dst)
		copied = append(copied, rel)
	}
	if len(copied) == 0 {
		t.Skipf("none of the sampled real plugins were found under %s", realInstall)
	}

	_, session, cleanup := connectInMemory(t)
	defer cleanup()

	initResult := callTool(t, session, "init_moodle_context", map[string]any{"moodle_path": root})
	if initResult.IsError {
		t.Fatalf("init_moodle_context: %s", toolText(t, initResult))
	}

	for _, rel := range copied {
		result := callTool(t, session, "generate_plugin_context", map[string]any{
			"plugin_path": rel, "format": "json",
		})
		if result.IsError {
			t.Fatalf("generate_plugin_context(%s): %s", rel, toolText(t, result))
		}
		var out struct {
			Component string `json:"component"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &out); err != nil {
			t.Fatalf("unmarshal(%s): %v", rel, err)
		}
		if out.Component == "" {
			t.Errorf("%s: empty component detected", rel)
		}

		content, err := os.ReadFile(generators.PluginOutputPath(filepath.Join(root, rel), "PLUGIN_AI_CONTEXT.md"))
		if err != nil {
			t.Fatalf("read PLUGIN_AI_CONTEXT.md for %s: %v", rel, err)
		}
		if !strings.Contains(string(content), out.Component) {
			t.Errorf("%s: PLUGIN_AI_CONTEXT.md doesn't mention its own component %q", rel, out.Component)
		}
		t.Logf("%s -> component=%s type=%s AI_CONTEXT.md=%d bytes", rel, out.Component, out.Type, len(content))
	}
}

// copyRealPluginTree copies the PHP files of the plugin directory `src` into `dst`, preserving the
// directory structure. Generated output and the tests, lang, amd, templates, pix and vendor
// directories are skipped. It fails the test on any error.
func copyRealPluginTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			base := d.Name()
			if base == "tests" || base == "lang" || base == "amd" || base == "templates" ||
				base == "pix" || base == ".build82" || base == "vendor" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !strings.HasSuffix(path, ".php") && d.Name() != "version.php" {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, rel)), 0o755); err != nil {
			return err
		}
		in, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer in.Close()
		out, createErr := os.Create(filepath.Join(dst, rel))
		if createErr != nil {
			return createErr
		}
		defer out.Close()
		_, copyErr := io.Copy(out, in)
		return copyErr
	})
	if err != nil {
		t.Fatalf("copying real plugin tree %s: %v", src, err)
	}
}
