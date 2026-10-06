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

package extractors

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

// The xml* types mirror the structure of a Moodle XMLDB install.xml document for decoding.
type xmlField struct {
	Name     string `xml:"NAME,attr"`
	Type     string `xml:"TYPE,attr"`
	Length   string `xml:"LENGTH,attr"`
	NotNull  string `xml:"NOTNULL,attr"`
	Default  string `xml:"DEFAULT,attr"`
	Sequence string `xml:"SEQUENCE,attr"`
	Comment  string `xml:"COMMENT,attr"`
}
type xmlKey struct {
	Name     string `xml:"NAME,attr"`
	Type     string `xml:"TYPE,attr"`
	Fields   string `xml:"FIELDS,attr"`
	RefTable string `xml:"REFTABLE,attr"`
}
type xmlIndex struct {
	Name   string `xml:"NAME,attr"`
	Unique string `xml:"UNIQUE,attr"`
	Fields string `xml:"FIELDS,attr"`
}
type xmlTable struct {
	Name    string     `xml:"NAME,attr"`
	Comment string     `xml:"COMMENT,attr"`
	Fields  []xmlField `xml:"FIELDS>FIELD"`
	Keys    []xmlKey   `xml:"KEYS>KEY"`
	Indexes []xmlIndex `xml:"INDEXES>INDEX"`
}
type xmlXMLDB struct {
	Tables []xmlTable `xml:"TABLES>TABLE"`
}

// DbSchema is the parsed content of a db/install.xml file.
type DbSchema struct {
	File   string
	Tables []DbTable
}

// DbTable is one table of a DbSchema with its fields, keys and indexes.
type DbTable struct {
	Name, Comment string
	Fields        []DbField
	Keys          []DbKey
	Indexes       []DbIndex
}

// DbField is one column of a DbTable.
type DbField struct {
	Name, Type, Length, Default, Comment string
	NotNull, Sequence                    bool
}

// DbKey is one key of a DbTable; Ref is the referenced table, when there is one.
type DbKey struct {
	Name, Type, Ref string
	Fields          []string
}

// DbIndex is one index of a DbTable.
type DbIndex struct {
	Name   string
	Unique bool
	Fields []string
}

// toBool reports whether the XMLDB attribute value `s` is true; only the literal "true" is.
func toBool(s string) bool {
	return s == "true"
}

// splitFields splits the comma-separated field list `s` and trims whitespace, dropping empty names.
func splitFields(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// mapFields converts decoded XML fields into DbField values.
func mapFields(fields []xmlField) []DbField {
	out := make([]DbField, 0, len(fields))
	for _, f := range fields {
		out = append(out, DbField{
			Name: f.Name, Type: f.Type, Length: f.Length, Default: f.Default, Comment: f.Comment,
			NotNull: toBool(f.NotNull), Sequence: toBool(f.Sequence),
		})
	}
	return out
}

// mapKeys converts decoded XML keys into DbKey values.
func mapKeys(keys []xmlKey) []DbKey {
	out := make([]DbKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, DbKey{Name: k.Name, Type: k.Type, Ref: k.RefTable, Fields: splitFields(k.Fields)})
	}
	return out
}

// mapIndexes converts decoded XML indexes into DbIndex values.
func mapIndexes(indexes []xmlIndex) []DbIndex {
	out := make([]DbIndex, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, DbIndex{Name: i.Name, Unique: toBool(i.Unique), Fields: splitFields(i.Fields)})
	}
	return out
}

// ParseInstallXml parses the db/install.xml file at `xmlFilePath`. It returns nil when the file
// cannot be read. Malformed XML yields a non-nil schema with an empty Tables slice.
func ParseInstallXml(xmlFilePath string) *DbSchema {
	content, err := readFileCapped(xmlFilePath)
	if err != nil {
		return nil
	}

	var doc xmlXMLDB
	if err := xml.Unmarshal(content, &doc); err != nil {
		return &DbSchema{File: xmlFilePath, Tables: []DbTable{}}
	}

	tables := make([]DbTable, 0, len(doc.Tables))
	for _, t := range doc.Tables {
		tables = append(tables, DbTable{
			Name: t.Name, Comment: t.Comment,
			Fields:  mapFields(t.Fields),
			Keys:    mapKeys(t.Keys),
			Indexes: mapIndexes(t.Indexes),
		})
	}
	return &DbSchema{File: xmlFilePath, Tables: tables}
}

// ExtractPluginSchema parses `pluginPath`/db/install.xml. It returns nil when that file cannot be
// read.
func ExtractPluginSchema(pluginPath string) *DbSchema {
	return ParseInstallXml(filepath.Join(pluginPath, "db", "install.xml"))
}

// TableToMarkdown renders one table as a heading, a fields table, and keys/indexes bullet lists.
func TableToMarkdown(table DbTable) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", table.Name)
	if table.Comment != "" {
		fmt.Fprintf(&b, "%s\n\n", table.Comment)
	}

	b.WriteString("| Field | Type | Length | Not Null | Default | Comment |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, f := range table.Fields {
		notNull := ""
		if f.NotNull {
			notNull = "yes"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", f.Name, f.Type, f.Length, notNull, f.Default, f.Comment)
	}
	b.WriteString("\n")

	b.WriteString("**Keys:**\n\n")
	if len(table.Keys) == 0 {
		b.WriteString("- _(none)_\n")
	}
	for _, k := range table.Keys {
		fmt.Fprintf(&b, "- `%s` (%s): %s\n", k.Name, k.Type, strings.Join(k.Fields, ", "))
	}
	b.WriteString("\n")

	b.WriteString("**Indexes:**\n\n")
	if len(table.Indexes) == 0 {
		b.WriteString("- _(none)_\n")
	}
	for _, idx := range table.Indexes {
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		fmt.Fprintf(&b, "- `%s`%s: %s\n", idx.Name, unique, strings.Join(idx.Fields, ", "))
	}

	return b.String()
}
