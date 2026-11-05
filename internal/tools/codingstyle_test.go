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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/phpcs"
	"github.com/oito2/mcp-build82/internal/phpcs/phpcstest"
)

// codingStyleFixture creates a Moodle root holding blocks/html (component block_html) with a few
// PHP files, points the configuration at it, installs the fake phpcs printing the recorded report
// `report` (a file under internal/phpcs/testdata, whose {{ROOT}} is the plugin directory), and
// returns the plugin directory and the file where the fake records its arguments.
func codingStyleFixture(t *testing.T, report string) (pluginPath, argsFile string) {
	t.Helper()
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	pluginPath = filepath.Join(moodlePath, "blocks", "html")
	for rel, content := range map[string]string{
		"version.php":    "<?php\n$plugin->component = 'block_html';\n$plugin->version = 2024010100;\n",
		"block_html.php": "<?php\nclass block_html {}\n",
		"db/access.php":  "<?php\n$capabilities = [];\n",
		"lib.php":        "<?php\n",
	} {
		full := filepath.Join(pluginPath, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	phpcstest.Install(t)
	reportPath, err := filepath.Abs(filepath.Join("..", "phpcs", "testdata", report))
	if err != nil {
		t.Fatal(err)
	}
	argsFile = filepath.Join(t.TempDir(), "args.json")
	t.Setenv("FAKE_PHPCS_REPORT", reportPath)
	t.Setenv("FAKE_PHPCS_ROOT", pluginPath)
	t.Setenv("FAKE_PHPCS_ARGS", argsFile)
	return pluginPath, argsFile
}

// fakePhpcsArgs returns the arguments the fake phpcs received.
func fakePhpcsArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got.Args
}

// TestCheckCodingStyle_ReportFromContainerPaths verifies the structured output built from a real
// phpcs report whose paths are those of a container (as with a wrapper script): files relative to
// the plugin and sorted, clean files left out, messages sorted by line and column with lower-case
// severities, and the whole plugin checked for errors only.
func TestCheckCodingStyle_ReportFromContainerPaths(t *testing.T) {
	pluginPath, argsFile := codingStyleFixture(t, "report_container.json")
	_, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "block_html"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Component != "block_html" || out.Path != "blocks/html" || out.Standard != "moodle" || out.Severity != "error" ||
		out.Errors != 4 || out.Warnings != 1 || out.Fixable != 2 || out.FilesChecked != 3 || out.Returned != 5 || out.Truncated {
		t.Errorf("unexpected totals %+v", out)
	}
	var files []string
	for _, f := range out.Files {
		files = append(files, f.File)
	}
	if !slices.Equal(files, []string{"block_html.php", "db/access.php"}) {
		t.Fatalf("files = %q", files)
	}
	var order []string
	for _, m := range out.Files[0].Messages {
		order = append(order, m.Severity+"@"+strconv.Itoa(m.Line)+":"+strconv.Itoa(m.Column))
	}
	if want := []string{"error@1:1", "error@27:5", "error@27:21", "warning@71:153"}; !slices.Equal(order, want) {
		t.Errorf("messages = %q, want %q", order, want)
	}
	if m := out.Files[0].Messages[1]; m.Source != "Squiz.Scope.MethodScope.Missing" || m.Message != `Visibility must be declared on method "init"` || m.Fixable {
		t.Errorf("unexpected message %+v", m)
	}
	args := fakePhpcsArgs(t, argsFile)
	if !slices.Contains(args, "--warning-severity=0") || !slices.Contains(args, "--standard=moodle") || args[len(args)-1] != pluginPath {
		t.Errorf("unexpected phpcs arguments %q", args)
	}
}

// TestCheckCodingStyle_ReportFromLocalPaths verifies a report whose paths are the plugin's own.
func TestCheckCodingStyle_ReportFromLocalPaths(t *testing.T) {
	codingStyleFixture(t, "report_root.json")
	_, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "blocks/html"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].File != "lib.php" || out.Files[0].Messages[0].Source != "moodle.Commenting.MissingDocblock.Function" {
		t.Errorf("unexpected output %+v", out)
	}
}

// TestCheckCodingStyle_FilesAndSeverity verifies that `files` are passed as paths inside the
// plugin and that severity "warning" keeps warnings.
func TestCheckCodingStyle_FilesAndSeverity(t *testing.T) {
	pluginPath, argsFile := codingStyleFixture(t, "report_root.json")
	_, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{
		Plugin: "block_html", Severity: "warning", Files: []string{"lib.php", "db"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Severity != "warning" {
		t.Errorf("severity = %q", out.Severity)
	}
	args := fakePhpcsArgs(t, argsFile)
	if slices.Contains(args, "--warning-severity=0") {
		t.Errorf("warnings must be kept with severity warning: %q", args)
	}
	tail := args[len(args)-2:]
	if want := []string{filepath.Join(pluginPath, "lib.php"), filepath.Join(pluginPath, "db")}; !slices.Equal(tail, want) {
		t.Errorf("paths = %q, want %q", tail, want)
	}
}

// TestCheckCodingStyle_Limit verifies that at most `limit` messages are returned, files whose
// messages were all cut are left out, and the text report says so.
func TestCheckCodingStyle_Limit(t *testing.T) {
	codingStyleFixture(t, "report_container.json")
	for limit, want := range map[int][]string{2: {"block_html.php"}, 4: {"block_html.php"}, 5: {"block_html.php", "db/access.php"}} {
		res, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "block_html", Limit: limit})
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for _, f := range out.Files {
			files = append(files, f.File)
		}
		wantReturned := min(limit, 5)
		if !slices.Equal(files, want) || out.Returned != wantReturned || out.Truncated != (limit < 5) {
			t.Errorf("limit %d: files %q returned %d truncated %v", limit, files, out.Returned, out.Truncated)
		}
		if note := strings.Contains(textOf(res), "Showing "+strconv.Itoa(wantReturned)+" of 5 message(s)"); note != (limit < 5) {
			t.Errorf("limit %d: truncation note present = %v:\n%s", limit, note, textOf(res))
		}
	}
}

// TestCheckCodingStyle_TextReport verifies the Markdown report's totals and violation table.
func TestCheckCodingStyle_TextReport(t *testing.T) {
	codingStyleFixture(t, "report_container.json")
	res, _, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "block_html"})
	if err != nil {
		t.Fatal(err)
	}
	text := textOf(res)
	for _, want := range []string{
		"## Coding style: block_html (moodle standard, errors)",
		"4 error(s), 1 warning(s) in 3 file(s) checked; 2 fixable with phpcbf.",
		"### db/access.php (1 error(s), 0 warning(s))",
		"| 27 | 5 | error | Visibility must be declared on method \"init\" | `Squiz.Scope.MethodScope.Missing` |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}

// TestBuildCodingStyleOutput_SortsFiles verifies that files come out sorted by path whatever the
// order of the report's file map.
func TestBuildCodingStyleOutput_SortsFiles(t *testing.T) {
	report := &phpcs.Report{Totals: &phpcs.Totals{Errors: 5}, Files: map[string]*phpcs.FileReport{}}
	for _, f := range []string{"e.php", "a.php", "d/x.php", "c.php", "b.php"} {
		report.Files["/m/local/x/"+f] = &phpcs.FileReport{Errors: 1, Messages: []phpcs.Message{{Type: "ERROR", Line: 1, Column: 1}}}
	}
	for range 20 {
		var files []string
		for _, f := range buildCodingStyleOutput(report, "/m/local/x", "local/x", 100).Files {
			files = append(files, f.File)
		}
		if want := []string{"a.php", "b.php", "c.php", "d/x.php", "e.php"}; !slices.Equal(files, want) {
			t.Fatalf("files = %q, want %q", files, want)
		}
	}
}

// TestCheckCodingStyle_NoFileCheckedIsAnError verifies that a report covering no file is an error
// rather than a clean result.
func TestCheckCodingStyle_NoFileCheckedIsAnError(t *testing.T) {
	codingStyleFixture(t, "report_empty.json")
	res, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "block_html"})
	if err == nil || !strings.Contains(err.Error(), "phpcs checked no PHP file") {
		t.Fatalf("expected an error, got %v (%+v, %v)", err, out, res)
	}
}

// TestRenderCodingStyleReport_EscapesPipes verifies that a "|" in a message does not break the
// violation table.
func TestRenderCodingStyleReport_EscapesPipes(t *testing.T) {
	text := renderCodingStyleReport(CheckCodingStyleOutput{
		Component: "local_x", Standard: "moodle", Severity: "error", Errors: 1, FilesChecked: 1,
		Files: []CodingStyleFile{{File: "lib.php", Errors: 1, Messages: []CodingStyleMessage{
			{Line: 3, Column: 1, Severity: "error", Source: "Generic.X", Message: "Expected \"a|b\""},
		}}},
	})
	if !strings.Contains(text, "| 3 | 1 | error | Expected \"a\\|b\" | `Generic.X` |") {
		t.Errorf("pipe not escaped:\n%s", text)
	}
}

// TestCheckCodingStyle_Refusals verifies the argument, file and phpcs-availability errors.
func TestCheckCodingStyle_Refusals(t *testing.T) {
	outside := t.TempDir()
	cases := []struct {
		name  string
		setup func(t *testing.T, pluginPath string)
		in    CheckCodingStyleInput
		want  string
	}{
		{"bad severity", nil, CheckCodingStyleInput{Severity: "info"}, `severity must be "error" or "warning"`},
		{"absolute file", nil, CheckCodingStyleInput{Files: []string{outside}}, "relative to the plugin directory"},
		{"escaping file", nil, CheckCodingStyleInput{Files: []string{"../../../etc"}}, "outside the plugin directory"},
		{"missing file", nil, CheckCodingStyleInput{Files: []string{"nope.php"}}, "does not exist in the plugin"},
		{"symlink out of the plugin", func(t *testing.T, pluginPath string) {
			if err := os.Symlink(outside, filepath.Join(pluginPath, "link")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}, CheckCodingStyleInput{Files: []string{"link"}}, "outside the plugin directory"},
		{"phpcs missing", func(t *testing.T, _ string) { phpcstest.EmptyPath(t) }, CheckCodingStyleInput{}, "composer global require moodlehq/moodle-cs"},
		{"standard missing", func(t *testing.T, _ string) { t.Setenv("FAKE_PHPCS_STANDARDS", "PEAR and PSR12") }, CheckCodingStyleInput{}, "moodle coding standard is not installed"},
		{"phpcs broken", func(t *testing.T, _ string) { t.Setenv("FAKE_PHPCS_MODE", "fail") }, CheckCodingStyleInput{}, "does not work"},
		{"no report", func(t *testing.T, _ string) { t.Setenv("FAKE_PHPCS_MODE", "garbage") }, CheckCodingStyleInput{}, "phpcs produced no report"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pluginPath, _ := codingStyleFixture(t, "report_root.json")
			if c.setup != nil {
				c.setup(t, pluginPath)
			}
			c.in.Plugin = "block_html"
			res, _, err := handleCheckCodingStyle(context.Background(), nil, c.in)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error containing %q, got %v (%v)", c.want, err, res)
			}
		})
	}
}

// TestPluginRelative verifies the mapping of reported paths to plugin-relative paths.
func TestPluginRelative(t *testing.T) {
	plugin := filepath.Join(t.TempDir(), "blocks", "html")
	cases := map[string]string{
		filepath.Join(plugin, "db", "access.php"):         "db/access.php",
		"/var/www/html/blocks/html/classes/output/x.php":  "classes/output/x.php",
		`C:\moodle\blocks\html\lib.php`:                   "lib.php",
		"/elsewhere/blocks/html/blocks/html/inner.php":    "inner.php",
		"/somewhere/else/entirely.php":                    "entirely.php",
		filepath.Join(filepath.Dir(plugin), "other", "a"): "a",
	}
	if runtime.GOOS == "windows" {
		delete(cases, "/var/www/html/blocks/html/classes/output/x.php")
	}
	for reported, want := range cases {
		if got := pluginRelative(reported, plugin, "blocks/html"); got != want {
			t.Errorf("pluginRelative(%q) = %q, want %q", reported, got, want)
		}
	}
}

// TestCheckCodingStyle_RealPhpcs runs the real phpcs, when it is installed with the moodle
// standard, on the demo fixture plugin and checks that a report is produced.
func TestCheckCodingStyle_RealPhpcs(t *testing.T) {
	if phpcs.Check(context.Background()).State != phpcs.Ready {
		t.Skip("phpcs with the moodle standard is not installed")
	}
	moodlePath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "moodle"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	_, out, err := handleCheckCodingStyle(context.Background(), nil, CheckCodingStyleInput{Plugin: "local_demo", Severity: "warning"})
	if err != nil {
		t.Fatal(err)
	}
	if out.FilesChecked == 0 {
		t.Errorf("expected phpcs to check the demo plugin's files, got %+v", out)
	}
}

// TestDoctor_PhpcsDependency verifies the doctor system-dependency check of phpcs in each state.
func TestDoctor_PhpcsDependency(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T)
		status checkStatus
		want   string
	}{
		{"ready", func(t *testing.T) { phpcstest.Install(t) }, statusOK, "with the moodle coding standard"},
		{"no standard", func(t *testing.T) {
			phpcstest.Install(t)
			t.Setenv("FAKE_PHPCS_STANDARDS", "PEAR and PSR12")
		}, statusWarn, "moodle coding standard is not installed"},
		{"unusable", func(t *testing.T) {
			phpcstest.Install(t)
			t.Setenv("FAKE_PHPCS_MODE", "fail")
		}, statusWarn, "does not work"},
		{"missing", phpcstest.EmptyPath, statusWarn, "not found (optional"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup(t)
			r := phpcsDependency()
			if r.Label != "phpcs" || r.Status != c.status || !strings.Contains(r.Detail, c.want) {
				t.Errorf("got %+v, want %s containing %q", r, c.status, c.want)
			}
		})
	}
}
