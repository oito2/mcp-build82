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

package prompts

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// setupUpgradePlugin creates a Moodle root (version 4.1, with an API index marking
// old_core_fn() deprecated) holding local/demo with a call to old_core_fn() in lib.php, a legacy
// before_footer callback, `supported` and `requires` in version.php, and an upgrade step without a
// savepoint. It points the configuration at the root and returns the plugin path.
func setupUpgradePlugin(t *testing.T) string {
	t.Helper()
	root, pluginPath := setupPromptPlugin(t)
	t.Setenv("BUILD82_MOODLE_VERSION", "4.1")
	mustMkdirAll(t, filepath.Join(root, ".build82"))
	mustWriteFile(t, filepath.Join(root, ".build82", "MOODLE_API_INDEX.md"),
		"# Moodle API Index\n\n- `old_core_fn()` ~~**@deprecated**: use new_core_fn()~~\n- `new_core_fn()` — current\n")
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010200;\n$plugin->requires = 2022112800;\n$plugin->supported = [401, 405];\n")
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"),
		"<?php\nfunction local_demo_before_footer() {\n    return old_core_fn();\n}\n\nfunction local_demo_other() {\n    $x = new_core_fn();\n    $y = $obj->old_core_fn();\n    return old_core_fn($x);\n}\n")
	mustMkdirAll(t, filepath.Join(pluginPath, "db"))
	mustWriteFile(t, filepath.Join(pluginPath, "db", "upgrade.php"),
		"<?php\nfunction xmldb_local_demo_upgrade($oldversion) {\n    if ($oldversion < 2024010200) {\n        $x = 1;\n    }\n    return true;\n}\n")
	return pluginPath
}

// renderUpgrade renders upgrade_plugin with `args` and returns its single message's text.
func renderUpgrade(t *testing.T, args map[string]string) string {
	t.Helper()
	result, err := handleUpgradePrompt(context.Background(), promptReq(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	return textContentOf(t, result.Messages[0])
}

// TestHandleUpgradePrompt_ContentReflectsPluginAnalysis verifies the metadata table, the
// deprecated calls (bare calls only, sorted by line), the legacy callback, the upgrade-step finding,
// the note about the installed API index, and the migration instructions.
func TestHandleUpgradePrompt_ContentReflectsPluginAnalysis(t *testing.T) {
	setupUpgradePlugin(t)
	text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": "4.5"})
	for _, want := range []string{
		"| Component | local_demo |",
		"| Plugin version | 2024010200 |",
		"| $plugin->requires | 2022112800 |",
		"| $plugin->supported | [401, 405] |",
		"| Installed Moodle | 4.1 |",
		"| Target Moodle | 4.5 (branch 405) |",
		"### Deprecated Core API Calls (2)\n\n| File | Line | Function |\n|---|---|---|\n| lib.php | 3 | `old_core_fn()` |\n| lib.php | 9 | `old_core_fn()` |\n",
		"### Legacy Callbacks With a Hook API Replacement (1)",
		"| `local_demo_before_footer` | `\\core\\hook\\output\\before_footer_html_generation` |",
		"### Upgrade-Step Findings (1)\n\n- **fail:** ",
		"has no savepoint call",
		"comes from the installed Moodle (4.1), not from 4.5",
		"`UPGRADING.md`",
		"`$plugin->supported` so its range includes 405",
		"`bump_plugin_version` tool",
		"PLUGIN AI CONTEXT BODY",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing %q:\n%s", want, text)
		}
	}
}

// TestHandleUpgradePrompt_BranchNumbers verifies the branch derived from target_version.
func TestHandleUpgradePrompt_BranchNumbers(t *testing.T) {
	setupUpgradePlugin(t)
	for target, branch := range map[string]string{"4.5": "405", "5.0": "500", "5.1": "501", "4.5.3": "405", " 5.2 ": "502", "10.0": "1000"} {
		text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": target})
		if want := "(branch " + branch + ")"; !strings.Contains(text, want) {
			t.Errorf("%q: missing %q", target, want)
		}
	}
}

// TestHandleUpgradePrompt_CleanPlugin verifies the "none found" sections of a plugin with nothing
// to report.
func TestHandleUpgradePrompt_CleanPlugin(t *testing.T) {
	setupPromptPlugin(t)
	text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": "5.0"})
	for _, want := range []string{
		"### Deprecated Core API Calls (0)\n\nNone found",
		"### Legacy Callbacks With a Hook API Replacement (0)\n\nNone found.",
		"### Upgrade-Step Findings (0)\n\nNone:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"| Installed Moodle |", "| $plugin->supported |", "| $plugin->requires |"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("an unknown value must not be listed (%q):\n%s", unwanted, text)
		}
	}
}

// TestHandleUpgradePrompt_NoAnalysisOutsideMoodleOrWithoutConfig verifies that no plugin file is
// analyzed when the plugin escapes the Moodle root or build82 is not configured.
func TestHandleUpgradePrompt_NoAnalysisOutsideMoodleOrWithoutConfig(t *testing.T) {
	pluginPath := setupUpgradePlugin(t)
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "version.php"), "<?php\n$plugin->component = 'local_secret';\n$plugin->version = 1;\n")
	mustWriteFile(t, filepath.Join(outside, "lib.php"), "<?php\nfunction local_secret_before_footer() { return old_core_fn(); }\n")

	// A version.php in the working directory must not make an unresolved plugin look analyzable.
	cwd := t.TempDir()
	mustWriteFile(t, filepath.Join(cwd, "version.php"), "<?php\n$plugin->version = 1;\n")
	t.Chdir(cwd)
	text := renderUpgrade(t, map[string]string{"plugin": outside, "target_version": "4.5"})
	if !strings.Contains(text, "### Plugin Analysis\n\nNot available") || strings.Contains(text, "Deprecated Core API Calls") || strings.Contains(text, "local_secret_before_footer") {
		t.Errorf("a plugin outside the Moodle root must not be analyzed:\n%s", text)
	}

	text = renderUpgrade(t, map[string]string{"plugin": "local_missing", "target_version": "4.5"})
	if !strings.Contains(text, "### Plugin Analysis\n\nNot available") || strings.Contains(text, "Deprecated Core API Calls") {
		t.Errorf("a plugin that does not exist must not be reported as analyzed:\n%s", text)
	}

	t.Setenv("BUILD82_MOODLE_PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	text = renderUpgrade(t, map[string]string{"plugin": pluginPath, "target_version": "4.5"})
	if !strings.Contains(text, "Not available") || strings.Contains(text, "| $plugin->supported |") || strings.Contains(text, "Deprecated Core API Calls") {
		t.Errorf("without a configuration the plugin must not be analyzed:\n%s", text)
	}
}

// TestHandleUpgradePrompt_InvalidArguments verifies the invalid-params errors.
func TestHandleUpgradePrompt_InvalidArguments(t *testing.T) {
	setupUpgradePlugin(t)
	for _, args := range []map[string]string{
		{"target_version": "4.5"},
		{"plugin": "local_demo"},
		{"plugin": "local_demo", "target_version": "  "},
		{"plugin": "local_demo", "target_version": "latest"},
		{"plugin": "local_demo", "target_version": "405"},
		{"plugin": "local_demo", "target_version": "4.5 | x"},
		{"plugin": "local_demo", "target_version": "0.5"},
		{"plugin": "local_demo", "target_version": "4.5.1.2"},
	} {
		_, err := handleUpgradePrompt(context.Background(), promptReq(args))
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
			t.Errorf("%v: expected an invalid-params error, got %v", args, err)
		}
	}
	for args, want := range map[string]map[string]string{
		"missing required argument(s): target_version": {"plugin": "local_demo"},
		"missing required argument(s): plugin":         {"target_version": "4.5"},
		`invalid target_version "latest"`:              {"plugin": "local_demo", "target_version": "latest"},
	} {
		if _, err := handleUpgradePrompt(context.Background(), promptReq(want)); err == nil || !strings.Contains(err.Error(), args) {
			t.Errorf("%v: expected %q, got %v", want, args, err)
		}
	}
}

// TestHandleUpgradePrompt_CapsDeprecatedRows verifies that the deprecated-call table stops at
// maxUpgradeDeprecatedRows rows with a count of the rest.
func TestHandleUpgradePrompt_CapsDeprecatedRows(t *testing.T) {
	pluginPath := setupUpgradePlugin(t)
	var b strings.Builder
	b.WriteString("<?php\nfunction local_demo_many() {\n")
	for range maxUpgradeDeprecatedRows + 7 {
		b.WriteString("    old_core_fn();\n")
	}
	b.WriteString("}\n")
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), b.String())

	text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": "4.5"})
	if got := strings.Count(text, "| `old_core_fn()` |"); got != maxUpgradeDeprecatedRows {
		t.Errorf("table has %d rows, want %d", got, maxUpgradeDeprecatedRows)
	}
	if !strings.Contains(text, fmt.Sprintf("### Deprecated Core API Calls (%d)", maxUpgradeDeprecatedRows+7)) || !strings.Contains(text, "...and 7 more.") {
		t.Errorf("missing total or remainder note:\n%s", text[len(text)-2000:])
	}
}

// TestHandleUpgradePrompt_EscapesTableCells verifies that a "|" in a value from version.php does
// not break the metadata table.
func TestHandleUpgradePrompt_EscapesTableCells(t *testing.T) {
	pluginPath := setupUpgradePlugin(t)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010200;\n$plugin->supported = ['a|b', 405];\n")
	text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": "4.5"})
	if !strings.Contains(text, "| $plugin->supported | ['a\\|b', 405] |") {
		t.Errorf("pipe not escaped in the metadata table:\n%s", text)
	}
}

// TestHandleUpgradePrompt_CapsFindings verifies that the upgrade-step findings stop at
// maxUpgradeFindingRows with a count of the rest.
func TestHandleUpgradePrompt_CapsFindings(t *testing.T) {
	pluginPath := setupUpgradePlugin(t)
	var b strings.Builder
	b.WriteString("<?php\nfunction xmldb_local_demo_upgrade($oldversion) {\n")
	for i := range maxUpgradeFindingRows + 3 {
		fmt.Fprintf(&b, "    if ($oldversion < %d) {\n        $x = 1;\n    }\n", 2023010100+i)
	}
	b.WriteString("    return true;\n}\n")
	mustWriteFile(t, filepath.Join(pluginPath, "db", "upgrade.php"), b.String())

	text := renderUpgrade(t, map[string]string{"plugin": "local_demo", "target_version": "4.5"})
	if got := strings.Count(text, "- **fail:** "); got != maxUpgradeFindingRows {
		t.Errorf("listed %d findings, want %d", got, maxUpgradeFindingRows)
	}
	if !strings.Contains(text, fmt.Sprintf("### Upgrade-Step Findings (%d)", maxUpgradeFindingRows+3)) || !strings.Contains(text, "- ...and 3 more.") {
		t.Errorf("missing total or remainder note")
	}
}

// TestHandleUpgradePrompt_MoodleRootIsNotAPlugin verifies that the Moodle root itself, which has a
// version.php of its own, is not analyzed as a plugin.
func TestHandleUpgradePrompt_MoodleRootIsNotAPlugin(t *testing.T) {
	root, _ := setupPromptPlugin(t)
	mustWriteFile(t, filepath.Join(root, "version.php"), "<?php\n$version = 2024100700;\n")
	for _, id := range []string{".", root} {
		text := renderUpgrade(t, map[string]string{"plugin": id, "target_version": "4.5"})
		if !strings.Contains(text, "### Plugin Analysis\n\nNot available") {
			t.Errorf("%q: the Moodle root was analyzed as a plugin:\n%s", id, text)
		}
	}
}
