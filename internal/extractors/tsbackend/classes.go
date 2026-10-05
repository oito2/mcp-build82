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

package tsbackend

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ExtractClasses walks dirPath recursively for *.php files and extracts every class/interface/
// trait/enum declaration, sorted by namespace then name — same contract as the regex backend.
// globPattern is accepted for API fidelity only; this implementation always walks recursively.
//
// Unlike the regex backend, there is no multi-line declaration joining here — a
// class_declaration/interface_declaration/trait_declaration/enum_declaration node's span already
// covers the whole construct regardless of how many lines its signature spans.
func ExtractClasses(dirPath, rootPath, globPattern string) phptypes.ClassesExtraction {
	if rootPath == "" {
		rootPath = dirPath
	}

	var files []string
	_ = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		// Skip symlinked files — os.ReadFile (in ParseFile) follows a symlink to wherever it
		// points, so a symlinked "foo.php" planted inside a scanned plugin directory would
		// otherwise get its target's content parsed into the generated classes index.
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(d.Name(), ".php") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return ExtractClassesFromFiles(files, rootPath)
}

// ExtractClassesFromFiles scans exactly the given files instead of walking a directory recursively
// — callers that already resolved a restricted glob (e.g. **/classes/**/*.php across a whole Moodle
// install) avoid parsing every *.php in the tree just to discard everything outside it.
func ExtractClassesFromFiles(files []string, rootPath string) phptypes.ClassesExtraction {
	var classes []phptypes.PhpClass
	for _, path := range files {
		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			rel = path
		}
		classes = append(classes, scanPhpClassFile(path, filepath.ToSlash(rel))...)
	}

	sort.Slice(classes, func(i, j int) bool {
		if classes[i].Namespace != classes[j].Namespace {
			return classes[i].Namespace < classes[j].Namespace
		}
		return classes[i].Name < classes[j].Name
	})
	return phptypes.ClassesExtraction{Classes: classes}
}

// scanPhpClassFile parses one PHP file and extracts every class-like declaration in it. Like the
// regex backend, every declaration in a file shares that file's single (first-found) namespace;
// files with multiple namespace blocks aren't scoped precisely.
func scanPhpClassFile(path, relFile string) []phptypes.PhpClass {
	tree, src, err := ParseFile(path)
	if err != nil {
		return nil
	}
	defer tree.Release()
	root := tree.RootNode()

	// Scope the search to namespace_definition's own namespace_name child specifically — a bare
	// "namespace_name" node also appears inside any qualified_name (e.g. an unrelated `implements
	// \some\iface` reference), so searching for that type anywhere in the file would pick up the
	// wrong one.
	namespace := ""
	if nsDef := FindDescendant(root, func(n *gotreesitter.Node) bool { return n.Type(phpLang) == "namespace_definition" }); nsDef != nil {
		if nsNameNode := FirstChildOfType(nsDef, "namespace_name"); nsNameNode != nil {
			namespace = nsNameNode.Text(src)
		}
	}

	var classes []phptypes.PhpClass
	for _, decl := range findAllClassLikeDeclarations(root) {
		nameNode := FirstChildOfType(decl, "name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Text(src)
		fqn := `\` + name
		if namespace != "" {
			fqn = `\` + namespace + `\` + name
		}
		classes = append(classes, phptypes.PhpClass{
			Name: name, Namespace: namespace, FQN: fqn, Kind: classKind(decl),
			File: relFile, Extends: classExtends(decl, src), Implements: classImplements(decl, src),
		})
	}
	return classes
}

func findAllClassLikeDeclarations(root *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	gotreesitter.Walk(root, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		switch n.Type(phpLang) {
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
			found = append(found, n)
		}
		return gotreesitter.WalkContinue
	})
	return found
}

func classKind(decl *gotreesitter.Node) phptypes.ClassKind {
	switch decl.Type(phpLang) {
	case "interface_declaration":
		return "interface"
	case "trait_declaration":
		return "trait"
	case "enum_declaration":
		return "enum"
	default: // class_declaration
		if FirstChildOfType(decl, "abstract_modifier") != nil {
			return "abstract class"
		}
		return "class"
	}
}

func classExtends(decl *gotreesitter.Node, src []byte) string {
	base := FirstChildOfType(decl, "base_clause")
	if base == nil || base.NamedChildCount() == 0 {
		return ""
	}
	return base.NamedChild(0).Text(src)
}

func classImplements(decl *gotreesitter.Node, src []byte) []string {
	clause := FirstChildOfType(decl, "class_interface_clause")
	if clause == nil {
		return nil
	}
	var implements []string
	for i := 0; i < clause.NamedChildCount(); i++ {
		implements = append(implements, clause.NamedChild(i).Text(src))
	}
	return implements
}

// --- renamedclasses.php ----------------------------------------------------------------------

// ParseRenamedClassesPhp parses a db/renamedclasses.php file's $renamedclasses map. Returns nil if
// the file can't be read or doesn't define the array.
func ParseRenamedClassesPhp(filePath string) []phptypes.RenamedClass {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()
	arr := FindAssignment(tree.RootNode(), MatchVariable(src, "renamedclasses"))
	if arr == nil {
		return nil
	}

	var out []phptypes.RenamedClass
	for _, element := range ArrayElements(arr) {
		key, value := KeyValue(element)
		oldName, ok := StringValue(key, src)
		if !ok {
			continue
		}
		newName := renamedClassValue(value, src)
		if newName == "" {
			continue
		}
		out = append(out, phptypes.RenamedClass{OldName: oldName, NewName: newName})
	}
	return out
}

// renamedClassValue accepts either a bare `Ns\Class::class` reference
// (class_constant_access_expression — real PHP code, nothing to unescape) or a quoted string
// literal (StringValue already unescapes it), matching the regex backend's own
// normalizeRenamedClassValue.
func renamedClassValue(node *gotreesitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	var raw string
	if node.Type(phpLang) == "class_constant_access_expression" && node.NamedChildCount() > 0 {
		raw = node.NamedChild(0).Text(src)
	} else if s, ok := StringValue(node, src); ok {
		raw = s
	} else {
		return ""
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "::class")
	raw = strings.TrimSpace(raw)
	return strings.TrimPrefix(raw, `\`)
}
