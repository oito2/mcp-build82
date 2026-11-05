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
	"path/filepath"
	"regexp"
	"sort"

	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/phparray"
)

// UpgradeSavepoint is one upgrade_*_savepoint() call of an upgrade step: the function name, the
// version argument as written, the string arguments after it (the module/block name, or the plugin
// type and name), and its 1-based line in db/upgrade.php.
type UpgradeSavepoint struct {
	Function string
	Version  string
	Args     []string
	Line     int
}

// TableField names one field of one table.
type TableField struct {
	Table, Field string
}

// UpgradeOp is one database-manager schema operation of an upgrade step, in source order: Op is
// create_table, drop_table, rename_table, add_field, drop_field or rename_field; Field is empty for
// the table operations; NewName is the new table or field name of a rename.
type UpgradeOp struct {
	Op, Table, Field, NewName string
	Line                      int
}

// UpgradeBlock is one `if ($oldversion < V) { ... }` step of db/upgrade.php with the schema
// operations it performs, in source order. Table and field names are resolved from the
// xmldb_table/xmldb_field variables of the step; an operation whose names cannot be resolved is
// not listed.
type UpgradeBlock struct {
	Version    string
	Line       int
	Savepoints []UpgradeSavepoint
	Ops        []UpgradeOp
	// Literals is the set of single- or double-quoted strings written in the step, so a caller can
	// tell that a later step refers to a table or field by name (for example through a helper
	// function this extractor does not follow).
	Literals map[string]bool
}

// UpgradeOperations is the parsed content of a plugin's db/upgrade.php: its steps in file order.
type UpgradeOperations struct {
	File   string
	Blocks []UpgradeBlock
}

// Patterns of the upgrade operations: the step opening, xmldb_table/xmldb_field assignments,
// database manager calls and savepoint calls.
var (
	upgradeStepOpenPattern = regexp.MustCompile(`if\s*\(\s*\$oldversion\s*<\s*([0-9]+(?:\.[0-9]+)?)\s*\)\s*\{`)
	xmldbAssignPattern     = regexp.MustCompile(`\$(\w+)\s*=\s*new\s+xmldb_(table|field)\s*\(\s*['"]([^'"]+)['"]`)
	dbmanCallPattern       = regexp.MustCompile(`\$dbman\s*->\s*(create_table|drop_table|rename_table|add_field|drop_field|rename_field)\s*\(\s*\$(\w+)\s*(?:,\s*(?:\$(\w+)|['"]([^'"]+)['"]))?(?:\s*,\s*(?:\$(\w+)|['"]([^'"]+)['"]))?`)
	stringAssignPattern    = regexp.MustCompile(`\$(\w+)\s*=\s*['"]([^'"]+)['"]\s*;`)
	foreachVarsPattern     = regexp.MustCompile(`foreach\s*\([^)]*?\bas\s+(?:&?\$(\w+)\s*=>\s*)?&?\$(\w+)\s*\)`)
	setNamePattern         = regexp.MustCompile(`\$(\w+)\s*->\s*setName\s*\(\s*(?:\$(\w+)|['"]([^'"]+)['"])\s*\)`)
	savepointCallPattern   = regexp.MustCompile(`\b(upgrade_\w+_savepoint)\s*\(\s*true\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*((?:,\s*['"][^'"]*['"]\s*)*)(?:,\s*(?:true|false|null)\s*)?\)`)
	quotedArgPattern       = regexp.MustCompile(`['"]([^'"]*)['"]`)
)

// upgradeStepLoc locates one version-gated upgrade step in a text: the offset of its `if`, the
// offsets of its body between the braces ([bodyStart, bodyEnd), bodyEnd being the closing brace),
// and the version it compares $oldversion with.
type upgradeStepLoc struct {
	start, bodyStart, bodyEnd int
	version                   string
}

// locateUpgradeSteps returns, in text order, every `if ($oldversion < V) {` step of `content` whose
// V is a Moodle version number (moodletype.IsUpgradeVersion) and whose body braces balance.
// `content` should have comments and heredocs blanked (stripCommentsAndHeredocs), so that a
// commented-out step or a brace inside a heredoc is not taken into account. ParseUpgradePhp and
// ParseUpgradeOperations both locate steps with it.
func locateUpgradeSteps(content string) []upgradeStepLoc {
	var steps []upgradeStepLoc
	for _, m := range upgradeStepOpenPattern.FindAllStringSubmatchIndex(content, -1) {
		version := content[m[2]:m[3]]
		if !moodletype.IsUpgradeVersion(version) {
			continue
		}
		braceIdx := m[1] - 1
		end := phparray.FindBalancedEnd(content, braceIdx, '{', '}')
		if end == -1 {
			continue
		}
		steps = append(steps, upgradeStepLoc{start: m[0], bodyStart: braceIdx + 1, bodyEnd: end, version: version})
	}
	return steps
}

// ParseUpgradeOperations reads `pluginPath`/db/upgrade.php and returns its upgrade steps with their
// savepoint calls and schema operations, in file order. Comments and heredocs are ignored. It
// returns nil when the file does not exist or cannot be read, and an UpgradeOperations with no
// blocks when it has no version-gated step. It always uses a regex scan regardless of
// BUILD82_EXTRACTOR_BACKEND.
func ParseUpgradeOperations(pluginPath string) *UpgradeOperations {
	path := filepath.Join(pluginPath, "db", "upgrade.php")
	raw, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	content := stripCommentsAndHeredocs(string(raw))
	ops := &UpgradeOperations{File: "db/upgrade.php"}
	lines := newLineIndex(content)

	for _, step := range locateUpgradeSteps(content) {
		block := UpgradeBlock{
			Version: step.version,
			Line:    lines.line(step.start),
		}
		parseUpgradeBody(&block, content, lines, step.bodyStart, step.bodyEnd)
		ops.Blocks = append(ops.Blocks, block)
	}
	return ops
}

// parseUpgradeBody fills `block` from content[start:end], the body of one upgrade step: savepoint
// calls with their lines, the quoted strings it contains, and the schema operations, resolving
// variables to the xmldb_table and xmldb_field names they hold at each call.
func parseUpgradeBody(block *UpgradeBlock, content string, lines lineIndex, start, end int) {
	body := content[start:end]

	block.Literals = map[string]bool{}
	for _, m := range quotedArgPattern.FindAllStringSubmatch(body, -1) {
		block.Literals[m[1]] = true
	}

	for _, m := range savepointCallPattern.FindAllStringSubmatchIndex(body, -1) {
		sp := UpgradeSavepoint{
			Function: body[m[2]:m[3]],
			Version:  body[m[4]:m[5]],
			Line:     lines.line(start + m[0]),
		}
		for _, a := range quotedArgPattern.FindAllStringSubmatch(body[m[6]:m[7]], -1) {
			sp.Args = append(sp.Args, a[1])
		}
		block.Savepoints = append(block.Savepoints, sp)
	}

	// foreachVars records where a foreach loop assigns a variable: from there on its value is not a
	// known literal or xmldb object.
	type loopAssign struct {
		pos      int
		variable string
	}
	var foreachVars []loopAssign
	for _, m := range foreachVarsPattern.FindAllStringSubmatchIndex(body, -1) {
		for _, g := range []int{2, 4} {
			if m[g] != -1 {
				foreachVars = append(foreachVars, loopAssign{m[0], body[m[g]:m[g+1]]})
			}
		}
	}

	// lastString returns the string literal held by $variable at pos: the latest literal assigned
	// to it, or "" when there is none or a foreach loop reassigned it afterwards.
	stringAssigns := stringAssignPattern.FindAllStringSubmatchIndex(body, -1)
	lastString := func(variable string, pos int) string {
		value, at := "", -1
		for _, m := range stringAssigns {
			if m[0] < pos && body[m[2]:m[3]] == variable {
				value, at = body[m[4]:m[5]], m[0]
			}
		}
		for _, f := range foreachVars {
			if f.variable == variable && f.pos > at && f.pos < pos {
				return ""
			}
		}
		return value
	}

	// naming is one event giving an xmldb variable its name: a `new xmldb_table/xmldb_field('n')`
	// assignment (kind table or field), a later `->setName(...)` call (kind "", name "" when it
	// cannot be resolved), or a foreach loop assigning the variable (kind "loop": name unknown).
	type naming struct {
		pos            int
		variable, kind string
		name           string
	}
	var namings []naming
	for _, m := range xmldbAssignPattern.FindAllStringSubmatchIndex(body, -1) {
		namings = append(namings, naming{pos: m[0], variable: body[m[2]:m[3]], kind: body[m[4]:m[5]], name: body[m[6]:m[7]]})
	}
	for _, m := range setNamePattern.FindAllStringSubmatchIndex(body, -1) {
		name := ""
		if m[4] != -1 {
			name = lastString(body[m[4]:m[5]], m[0])
		} else {
			name = body[m[6]:m[7]]
		}
		namings = append(namings, naming{pos: m[0], variable: body[m[2]:m[3]], kind: "", name: name})
	}
	for _, f := range foreachVars {
		namings = append(namings, naming{pos: f.pos, variable: f.variable, kind: "loop"})
	}
	sort.Slice(namings, func(a, b int) bool { return namings[a].pos < namings[b].pos })

	// resolve returns the name $variable holds as an xmldb_<kind> at pos: the name of its latest
	// assignment, or of a later setName call. It returns "" when the variable is unknown or its
	// name cannot be resolved, so the operation is left out rather than misreported.
	resolve := func(kind, variable string, pos int) string {
		name, known := "", false
		for _, n := range namings {
			if n.pos >= pos || n.variable != variable {
				continue
			}
			switch {
			case n.kind == kind:
				name, known = n.name, true
			case n.kind == "loop":
				name, known = "", false
			case n.kind == "" && known:
				name = n.name
			}
		}
		return name
	}

	for _, m := range dbmanCallPattern.FindAllStringSubmatchIndex(body, -1) {
		op := body[m[2]:m[3]]
		table := resolve("table", body[m[4]:m[5]], m[0])
		if table == "" {
			continue
		}
		group := func(i int) string {
			if m[2*i] == -1 {
				return ""
			}
			return body[m[2*i]:m[2*i+1]]
		}
		fieldVar, literal := group(3), group(4)
		third := group(6)
		if v := group(5); v != "" {
			third = lastString(v, m[0])
		}
		field := ""
		if fieldVar != "" {
			field = resolve("field", fieldVar, m[0])
		}
		line := lines.line(start + m[0])
		switch op {
		case "create_table", "drop_table":
			block.Ops = append(block.Ops, UpgradeOp{Op: op, Table: table, Line: line})
		case "rename_table":
			if literal != "" {
				block.Ops = append(block.Ops, UpgradeOp{Op: op, Table: table, NewName: literal, Line: line})
			}
		case "add_field", "drop_field":
			if field != "" {
				block.Ops = append(block.Ops, UpgradeOp{Op: op, Table: table, Field: field, Line: line})
			}
		case "rename_field":
			if field != "" && third != "" {
				block.Ops = append(block.Ops, UpgradeOp{Op: op, Table: table, Field: field, NewName: third, Line: line})
			}
		}
	}
}
