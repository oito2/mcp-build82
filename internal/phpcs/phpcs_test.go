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

package phpcs_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/phpcs"
	"github.com/oito2/mcp-build82/internal/phpcs/phpcstest"
)

// TestInstalledStandards verifies the parsing of `phpcs -i` output.
func TestInstalledStandards(t *testing.T) {
	cases := map[string][]string{
		"The installed coding standards are MySource, PEAR, PSR1, PSR2, PSR12, Squiz and Zend\n": {"MySource", "PEAR", "PSR1", "PSR2", "PSR12", "Squiz", "Zend"},
		"The installed coding standards are PSR12, moodle-extra and moodle":                      {"PSR12", "moodle-extra", "moodle"},
		"The only coding standard installed is moodle":                                           {"The only coding standard installed is moodle"},
		"": nil,
	}
	for in, want := range cases {
		if got := phpcs.InstalledStandards(in); !slices.Equal(got, want) {
			t.Errorf("InstalledStandards(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCheck verifies each availability state.
func TestCheck(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		phpcstest.EmptyPath(t)
		if s := phpcs.Check(context.Background()); s.State != phpcs.Missing {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("ready", func(t *testing.T) {
		phpcstest.Install(t)
		if s := phpcs.Check(context.Background()); s.State != phpcs.Ready {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("only moodle-extra is not the moodle standard", func(t *testing.T) {
		phpcstest.Install(t)
		t.Setenv("FAKE_PHPCS_STANDARDS", "PEAR, moodle-extra and Zend")
		if s := phpcs.Check(context.Background()); s.State != phpcs.NoStandard {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("unusable", func(t *testing.T) {
		phpcstest.Install(t)
		t.Setenv("FAKE_PHPCS_MODE", "fail")
		if s := phpcs.Check(context.Background()); s.State != phpcs.Unusable || !strings.Contains(s.Detail, "phpcs is broken") {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("hanging", func(t *testing.T) {
		phpcstest.Install(t)
		t.Setenv("FAKE_PHPCS_MODE", "hang")
		saved := phpcs.InfoTimeout
		phpcs.InfoTimeout = 300 * time.Millisecond
		t.Cleanup(func() { phpcs.InfoTimeout = saved })
		start := time.Now()
		if s := phpcs.Check(context.Background()); s.State != phpcs.Unusable || !strings.Contains(s.Detail, "did not finish") {
			t.Errorf("got %+v", s)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("Check took %s", d)
		}
	})
}

// TestRun_ParsesReportAndPassesArguments verifies that a report is parsed whatever phpcs's
// non-zero exit code, and the arguments and working directory phpcs receives.
func TestRun_ParsesReportAndPassesArguments(t *testing.T) {
	phpcstest.Install(t)
	report, _ := filepath.Abs(filepath.Join("testdata", "report_container.json"))
	argsFile := filepath.Join(t.TempDir(), "args.json")
	t.Setenv("FAKE_PHPCS_REPORT", report)
	t.Setenv("FAKE_PHPCS_ARGS", argsFile)
	dir := t.TempDir()

	for _, warnings := range []bool{false, true} {
		paths := []string{filepath.Join(dir, "-odd.php"), filepath.Join(dir, "b.php")}
		r, err := phpcs.Run(context.Background(), dir, paths, warnings)
		if err != nil {
			t.Fatal(err)
		}
		if r.Totals.Errors != 4 || r.Totals.Warnings != 1 || r.Totals.Fixable != 2 || len(r.Files) != 3 {
			t.Fatalf("unexpected report %+v", r)
		}
		f := r.Files["/var/www/html/blocks/html/block_html.php"]
		if f == nil || len(f.Messages) != 4 || f.Messages[3].Source != "Squiz.Scope.MethodScope.Missing" || f.Messages[3].Line != 27 || f.Messages[3].Column != 5 || f.Messages[3].Type != "ERROR" {
			t.Fatalf("unexpected file report %+v", f)
		}

		var got struct {
			Dir  string   `json:"dir"`
			Args []string `json:"args"`
		}
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if same, _ := sameDir(got.Dir, dir); !same {
			t.Errorf("phpcs ran in %s, want %s", got.Dir, dir)
		}
		want := []string{"--standard=moodle", "--report=json", "--extensions=php", "-q", "--ignore=*/.build82/*,*/vendor/*,*/node_modules/*"}
		if !warnings {
			want = append(want, "--warning-severity=0")
		}
		want = append(want, paths...)
		if !slices.Equal(got.Args, want) {
			t.Errorf("args (warnings=%v) = %q, want %q", warnings, got.Args, want)
		}
	}
}

// sameDir reports whether `a` and `b` name the same directory.
func sameDir(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

// TestRun_Failures verifies the errors of a phpcs that prints no report, is missing, or hangs.
func TestRun_Failures(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		phpcstest.Install(t)
		for _, p := range []string{"-odd.php", "--bootstrap=x.php", "lib.php"} {
			if _, err := phpcs.Run(context.Background(), t.TempDir(), []string{p}, false); err == nil || !strings.Contains(err.Error(), "must be absolute") {
				t.Errorf("%s: got %v, want a must-be-absolute error", p, err)
			}
		}
	})
	t.Run("no report", func(t *testing.T) {
		phpcstest.Install(t)
		t.Setenv("FAKE_PHPCS_MODE", "garbage")
		dir := t.TempDir()
		if _, err := phpcs.Run(context.Background(), dir, []string{dir}, false); err == nil || !strings.Contains(err.Error(), `the "moodle" coding standard is not installed`) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		phpcstest.EmptyPath(t)
		if _, err := phpcs.Run(context.Background(), t.TempDir(), []string{t.TempDir()}, false); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		phpcstest.Install(t)
		t.Setenv("FAKE_PHPCS_MODE", "hang")
		saved := phpcs.RunTimeout
		phpcs.RunTimeout = 300 * time.Millisecond
		t.Cleanup(func() { phpcs.RunTimeout = saved })
		start := time.Now()
		if _, err := phpcs.Run(context.Background(), t.TempDir(), []string{t.TempDir()}, false); !errors.Is(err, phpcs.ErrTimeout) {
			t.Errorf("got %v", err)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("Run took %s", d)
		}
	})
}

// installWrapperScript installs the fake phpcs behind a shell wrapper script that runs it as a
// child process, so killing the script leaves the child holding the output open.
func installWrapperScript(t *testing.T) {
	t.Helper()
	dir := phpcstest.Install(t)
	if err := os.Rename(filepath.Join(dir, "phpcs"), filepath.Join(dir, "realphpcs")); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n\"" + filepath.Join(dir, "realphpcs") + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "phpcs"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PHPCS_MODE", "hang")
	t.Setenv("FAKE_PHPCS_HANG", "20s")
	saved := *phpcs.WaitDelay
	*phpcs.WaitDelay = 500 * time.Millisecond
	t.Cleanup(func() { *phpcs.WaitDelay = saved })
}

// TestCheck_TimeoutWithWrapperScript verifies that a timeout ends `phpcs -i` even when phpcs is a
// wrapper script whose child process keeps the output open after the script is killed.
func TestCheck_TimeoutWithWrapperScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper scripts are a unix case")
	}
	installWrapperScript(t)
	saved := phpcs.InfoTimeout
	phpcs.InfoTimeout = 200 * time.Millisecond
	t.Cleanup(func() { phpcs.InfoTimeout = saved })

	start := time.Now()
	if s := phpcs.Check(context.Background()); s.State != phpcs.Unusable {
		t.Errorf("got %+v", s)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Check took %s: the orphaned child kept it waiting", d)
	}
}

// TestRun_TimeoutWithWrapperScript verifies that a timeout ends the check even when phpcs is a
// wrapper script whose child process keeps the output open after the script is killed.
func TestRun_TimeoutWithWrapperScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper scripts are a unix case")
	}
	installWrapperScript(t)
	saved := phpcs.RunTimeout
	phpcs.RunTimeout = 200 * time.Millisecond
	t.Cleanup(func() { phpcs.RunTimeout = saved })

	start := time.Now()
	if _, err := phpcs.Run(context.Background(), t.TempDir(), []string{t.TempDir()}, false); !errors.Is(err, phpcs.ErrTimeout) {
		t.Errorf("got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Run took %s: the orphaned child kept it waiting", d)
	}
}
