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
	"path/filepath"
	"strings"
	"testing"
)

const schemaFixtureWellFormed = `<?xml version="1.0" encoding="UTF-8" ?>
<XMLDB PATH="local/test/db" VERSION="20240101">
  <TABLES>
    <TABLE NAME="local_test_records" COMMENT="Test records">
      <FIELDS>
        <FIELD NAME="id"     TYPE="int"  LENGTH="10" NOTNULL="true" SEQUENCE="true"/>
        <FIELD NAME="name"   TYPE="char" LENGTH="255" NOTNULL="true"/>
        <FIELD NAME="value"  TYPE="text" NOTNULL="false"/>
      </FIELDS>
      <KEYS>
        <KEY NAME="primary" TYPE="primary" FIELDS="id"/>
      </KEYS>
      <INDEXES>
        <INDEX NAME="name_idx" UNIQUE="false" FIELDS="name"/>
      </INDEXES>
    </TABLE>
  </TABLES>
</XMLDB>`

const schemaFixtureMalformed = "this is not xml <<>>"

func TestParseInstallXml(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "install.xml"), schemaFixtureWellFormed)

	result := ParseInstallXml(filepath.Join(dir, "db", "install.xml"))
	if result == nil || len(result.Tables) != 1 {
		t.Fatalf("expected 1 table, got %+v", result)
	}
	table := result.Tables[0]
	if table.Name != "local_test_records" || len(table.Fields) != 3 || len(table.Keys) != 1 || len(table.Indexes) != 1 {
		t.Errorf("table shape mismatch: %+v", table)
	}
	idField := table.Fields[0]
	if idField.Name != "id" || idField.Type != "int" || !idField.NotNull || !idField.Sequence {
		t.Errorf("id field mismatch: %+v", idField)
	}
	valueField := table.Fields[2]
	if valueField.NotNull {
		t.Errorf("expected NOTNULL=\"false\" to map to false, got %+v", valueField)
	}
}

func TestParseInstallXml_Malformed(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "install.xml"), schemaFixtureMalformed)

	result := ParseInstallXml(filepath.Join(dir, "db", "install.xml"))
	if result == nil {
		t.Fatal("expected non-nil result for malformed XML, got nil")
	}
	if len(result.Tables) != 0 {
		t.Errorf("expected empty tables for malformed XML, got %d", len(result.Tables))
	}
}

func TestParseInstallXml_MissingFile(t *testing.T) {
	if ParseInstallXml("/nonexistent/db/install.xml") != nil {
		t.Error("expected nil for a missing file")
	}
}

func TestToBool_ExactStringTrueOnly(t *testing.T) {
	cases := map[string]bool{"true": true, "1": false, "TRUE": false, "": false, "false": false}
	for in, want := range cases {
		if got := toBool(in); got != want {
			t.Errorf("toBool(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTableToMarkdown(t *testing.T) {
	result := ParseInstallXml(mustWriteTempInstallXml(t))
	md := TableToMarkdown(result.Tables[0])

	for _, want := range []string{"### local_test_records", "id", "name_idx", "primary"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected markdown to contain %q, got:\n%s", want, md)
		}
	}
}

func mustWriteTempInstallXml(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	path := filepath.Join(dir, "db", "install.xml")
	mustWriteFile(t, path, schemaFixtureWellFormed)
	return path
}
