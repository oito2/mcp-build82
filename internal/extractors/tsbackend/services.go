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
	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ParseServicesPhp parses a db/services.php file's $functions map — keyed by function name (unlike
// events/tasks's positional array-of-arrays). Requires either classname or description to include
// an entry, matching the regex backend exactly.
func ParseServicesPhp(filePath string) *phptypes.ServicesExtraction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()

	functionsArray := FindAssignment(tree.RootNode(), MatchVariable(src, "functions"))
	if functionsArray == nil {
		return &phptypes.ServicesExtraction{File: filePath, Functions: []phptypes.WebServiceFunction{}}
	}

	functions := []phptypes.WebServiceFunction{}
	for _, entry := range ArrayElements(functionsArray) {
		key, inner := KeyValue(entry)
		name, ok := StringValue(key, src)
		if !ok || inner == nil {
			continue
		}
		fn, include := parseServiceFunction(name, inner, src)
		if include {
			functions = append(functions, fn)
		}
	}
	return &phptypes.ServicesExtraction{File: filePath, Functions: functions}
}

// parseServiceFunction reads one function definition's keyed fields. include is false when
// neither classname nor description is present — the caller skips the entry. methodname defaults
// "execute", type defaults "read", loginrequired defaults true.
func parseServiceFunction(name string, inner *gotreesitter.Node, src []byte) (phptypes.WebServiceFunction, bool) {
	fn := phptypes.WebServiceFunction{Name: name, MethodName: "execute", Type: "read", LoginRequired: true}
	haveClassname, haveDescription := false, false

	for _, element := range ArrayElements(inner) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		switch k {
		case "classname":
			if s, ok := StringValue(value, src); ok {
				fn.ClassName = s
				haveClassname = true
			}
		case "methodname":
			if s, ok := StringValue(value, src); ok {
				fn.MethodName = orDefault(s, "execute")
			}
		case "description":
			if s, ok := StringValue(value, src); ok {
				fn.Description = s
				haveDescription = true
			}
		case "type":
			if s, ok := StringValue(value, src); ok {
				fn.Type = orDefault(s, "read")
			}
		case "capabilities":
			fn.Capabilities, _ = StringValue(value, src)
		case "ajax":
			if b, ok := BoolValue(value, src); ok {
				fn.Ajax = b
			}
		case "loginrequired":
			if b, ok := BoolValue(value, src); ok {
				fn.LoginRequired = b
			}
		}
	}
	return fn, haveClassname || haveDescription
}
