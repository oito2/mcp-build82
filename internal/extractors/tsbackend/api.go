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

package tsbackend

import (
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phpdoc"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ExtractFunctionsFromPhpFile scans a single PHP file for top-level function declarations,
// returning every function unfiltered (deprecated/private/internal/unverified included), like the
// regex backend's scanPhpFile. Visibility classification is done by internal/phpdoc.ClassifyVisibility.
func ExtractFunctionsFromPhpFile(filePath string) []phptypes.ApiFunction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()
	root := tree.RootNode()
	file := filepath.Base(filePath)

	var results []phptypes.ApiFunction
	for i := 0; i < root.NamedChildCount(); i++ {
		fn := root.NamedChild(i)
		if fn.Type(phpLang) != "function_definition" {
			continue
		}
		nameNode := FirstChildOfType(fn, "name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Text(src)
		if strings.HasPrefix(name, "__") {
			continue
		}
		doc := precedingDocComment(fn, src)
		results = append(results, phptypes.ApiFunction{
			Name: name, File: file, FilePath: filePath,
			Visibility: phpdoc.ClassifyVisibility(name, doc),
			Doc:        doc,
			// nameNode's own line, not fn.StartPoint().Row: a PHP 8 attribute (`#[...]`) immediately
			// preceding a function is included in the function_definition node's own span, so
			// fn.StartPoint().Row would report the attribute's line instead of the `function`
			// keyword's line — the regex backend always reports the real `function` line, so this
			// keeps parity with it.
			Line: int(nameNode.StartPoint().Row) + 1,
		})
	}
	return results
}

// precedingDocComment finds fn's immediately preceding PHPDoc block, if any — mirrors the regex
// backend's own findDocBlock: only a `/* ... */`-style comment counts (not `//`), and up to 3
// blank source lines between the comment and the function are tolerated (a row gap of 1 to 4
// inclusive; the comment ends the line right before the function at gap 1).
func precedingDocComment(fn *gotreesitter.Node, src []byte) *phpdoc.PhpDocBlock {
	prev := fn.PrevSibling()
	if prev == nil || prev.Type(phpLang) != "comment" {
		return nil
	}
	text := prev.Text(src)
	if !strings.HasPrefix(text, "/*") {
		return nil // a "//" line comment never counts as a PHPDoc block
	}
	gap := int(fn.StartPoint().Row) - int(prev.EndPoint().Row)
	if gap < 1 || gap > 4 {
		return nil
	}
	doc := phpdoc.ParseDocBlock(text)
	return &doc
}
