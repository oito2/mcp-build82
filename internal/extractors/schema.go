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
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

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
type DbTable struct {
	Name, Comment string
	Fields        []DbField
	Keys          []DbKey
	Indexes       []DbIndex
}
type DbField struct {
	Name, Type, Length, Default, Comment string
	NotNull, Sequence                    bool
}
type DbKey struct {
	Name, Type, Ref string
	Fields          []string
}
type DbIndex struct {
	Name   string
	Unique bool
	Fields []string
}

// toBool applies Moodle XMLDB's exact truthiness rule: only the literal string "true" is true.
func toBool(s string) bool {
	return s == "true"
}

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

func mapKeys(keys []xmlKey) []DbKey {
	out := make([]DbKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, DbKey{Name: k.Name, Type: k.Type, Ref: k.RefTable, Fields: splitFields(k.Fields)})
	}
	return out
}

func mapIndexes(indexes []xmlIndex) []DbIndex {
	out := make([]DbIndex, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, DbIndex{Name: i.Name, Unique: toBool(i.Unique), Fields: splitFields(i.Fields)})
	}
	return out
}

// ParseInstallXml parses a db/install.xml file. Returns nil if the file doesn't exist. Malformed
// XML content degrades to a non-nil *DbSchema with an empty Tables slice — it never returns an
// error for malformed content, only for a missing file.
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

// ExtractPluginSchema parses pluginPath/db/install.xml.
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
