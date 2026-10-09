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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stdioPurityExceptions lists the only source files, relative to the module root, allowed to write
// to stdout. Each belongs to a CLI-only code path (help and version text, install and uninstall
// prompts, self-update progress, the release and link-check tools) that never runs in the same process as the
// MCP stdio server.
var stdioPurityExceptions = map[string]bool{
	filepath.Join("cmd", "build82", "main.go"):               true,
	filepath.Join("internal", "installer", "install.go"):     true,
	filepath.Join("internal", "installer", "uninstall.go"):   true,
	filepath.Join("internal", "selfupdate", "selfupdate.go"): true,
	filepath.Join("scripts", "release", "main.go"):           true, // release build tool, never runs as the MCP server
	filepath.Join("scripts", "linkcheck", "main.go"):         true, // documentation link checker, never runs as the MCP server
}

// stdoutCallPattern matches fmt.Print, fmt.Println, fmt.Printf and any use of os.Stdout.
var stdoutCallPattern = regexp.MustCompile(`\bfmt\.Print(ln|f)?\(|\bos\.Stdout\b`)

// TestStdioPurity_NoStrayStdoutWrites enforces that only the JSON-RPC protocol touches stdout in
// stdio mode. It scans every non-test Go source file in the module, except those listed in
// stdioPurityExceptions, for stdout writes, since one would corrupt the protocol stream.
func TestStdioPurity_NoStrayStdoutWrites(t *testing.T) {
	root := filepath.Join("..", "..")
	var violations []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".dev" || d.Name() == ".git" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if stdioPurityExceptions[rel] {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if stdoutCallPattern.Match(content) {
			violations = append(violations, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking module source: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found stdout writes outside the known CLI-only exceptions (breaks stdio JSON-RPC purity): %v", violations)
	}
}

// TestStdoutCallPattern_Detection checks stdoutCallPattern against snippets that must and must not
// match, so that weakening the pattern fails.
func TestStdoutCallPattern_Detection(t *testing.T) {
	positives := []string{
		`fmt.Println("hello")`,
		`fmt.Print("hello")`,
		`fmt.Printf("%s\n", name)`,
		`os.Stdout.Write(b)`,
		`fmt.Fprintln(os.Stdout, "hello")`,
	}
	for _, snippet := range positives {
		if !stdoutCallPattern.MatchString(snippet) {
			t.Errorf("expected a match (stdout write) for %q", snippet)
		}
	}

	negatives := []string{
		`fmt.Fprintln(os.Stderr, "hello")`,
		`fmt.Fprintf(w, "%s", name)`,
		`fmt.Sprintf("%s", name)`,
		`fmt.Errorf("boom: %w", err)`,
		`myPrintln()`,
		`log.Println("hello")`,
		`someStdoutHandler.Flush()`,
	}
	for _, snippet := range negatives {
		if stdoutCallPattern.MatchString(snippet) {
			t.Errorf("expected no match for %q, a legitimate non-stdout call", snippet)
		}
	}
}
