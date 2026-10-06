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

package tools

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/generators"
)

// TestSearchApiVisibilityFilter calls handleSearchApi against a real API index and verifies the
// public, deprecated and all visibility filters.
func TestSearchApiVisibilityFilter(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	mustMkdirAll(t, filepath.Join(moodlePath, generators.ContextDir))
	mustWriteFile(t, generators.GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md"),
		"# Moodle API Index\n\n- `zzz_public_fn()` — a public function\n- `zzz_old_fn()` ~~**@deprecated**: old~~\n")

	cases := []struct {
		vis        ApiVisibilityFilter
		want, deny []string
	}{
		{"", []string{"zzz_public_fn"}, []string{"zzz_old_fn"}},
		{ApiVisPublic, []string{"zzz_public_fn"}, []string{"zzz_old_fn"}},
		{ApiVisDeprecated, []string{"zzz_old_fn"}, []string{"zzz_public_fn"}},
		{ApiVisAll, []string{"zzz_public_fn", "zzz_old_fn"}, nil},
	}
	for _, c := range cases {
		res, _, err := handleSearchApi(context.Background(), nil, SearchApiInput{Query: "zzz_", Visibility: c.vis})
		if err != nil || res.IsError {
			t.Fatalf("visibility %q: err=%v result=%+v", c.vis, err, res)
		}
		text := resultText(t, res)
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("visibility %q: missing %q in:\n%s", c.vis, w, text)
			}
		}
		for _, d := range c.deny {
			if strings.Contains(text, d) {
				t.Errorf("visibility %q: unexpected %q in:\n%s", c.vis, d, text)
			}
		}
	}
}

// TestFilterTableRows_KeepsPluginsNamedComponent verifies only the header and separator rows are
// dropped, so a plugin whose row contains "Component" is still returned.
func TestFilterTableRows_KeepsPluginsNamedComponent(t *testing.T) {
	lines := []string{
		"| Component | Type | Name | Version | Path |",
		"|---|---|---|---|---|",
		"| local_component | local | component | 1 | local/component |",
		"not a row",
	}
	got := filterTableRows(lines)
	if len(got) != 1 || !strings.Contains(got[0], "local_component") {
		t.Fatalf("expected only the local_component row, got %v", got)
	}
}

// TestInstallXMLSkeleton_SchemaPathMatchesPluginDepth verifies the xmldb.xsd relative path is
// computed from the plugin's db/ directory for one-level and nested plugin types.
func TestInstallXMLSkeleton_SchemaPathMatchesPluginDepth(t *testing.T) {
	cases := []struct{ typeDir, want string }{
		{"local", `"../../../lib/xmldb/xmldb.xsd"`},
		{"admin/tool", `"../../../../lib/xmldb/xmldb.xsd"`},
	}
	for _, c := range cases {
		out := installXMLSkeleton(skeletonParams{typeDir: c.typeDir, name: "x", component: "x_x"})
		if !strings.Contains(out, c.want) {
			t.Errorf("typeDir %q: want %s in:\n%s", c.typeDir, c.want, out)
		}
	}
}

// TestCreateZip_SkipsSymlinksAndReportsThem verifies symbolic links to files and directories are
// not archived (nor their targets) and are returned as plugin-relative paths.
func TestCreateZip_SkipsSymlinksAndReportsThem(t *testing.T) {
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "secret.txt"), "TOPSECRET")
	mustMkdirAll(t, filepath.Join(outside, "dir"))
	mustWriteFile(t, filepath.Join(outside, "dir", "inner.txt"), "INNER")

	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\n")
	mustMkdirAll(t, filepath.Join(pluginPath, "sub"))
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(pluginPath, "sub", "link.txt")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(pluginPath, "linkdir")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	_, links, err := createZip(pluginPath, dest, "myplugin", excludedNames)
	if err != nil {
		t.Fatalf("createZip: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 skipped links, got %v", links)
	}
	joined := strings.Join(links, ",")
	if !strings.Contains(joined, "sub/link.txt") || !strings.Contains(joined, "linkdir") || strings.Contains(joined, outside) {
		t.Errorf("unexpected links report: %v", links)
	}

	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if len(names) != 1 || names[0] != "myplugin/lib.php" {
		t.Errorf("expected only myplugin/lib.php, got %v", names)
	}
}
