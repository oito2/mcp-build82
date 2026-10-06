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

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// WebServiceFunction and ServicesExtraction alias the phptypes types of the same name.
type WebServiceFunction = phptypes.WebServiceFunction
type ServicesExtraction = phptypes.ServicesExtraction

// functionKeyPattern matches a quoted function name followed by the opening of its array.
var functionKeyPattern = regexp.MustCompile(`['"]([a-zA-Z0-9_]+)['"]\s*=>\s*(\[|array\s*\()`)

// ParseServicesPhp parses the $functions array of the db/services.php file at `filePath`. Entries
// with neither a classname nor a description are skipped; methodname defaults to "execute", type
// to "read" and loginrequired to true. It returns nil when the file cannot be read, and an empty
// extraction when the file has no $functions array.
func ParseServicesPhp(filePath string) *ServicesExtraction {
	if useTreesitter() {
		return tsbackend.ParseServicesPhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "functions")
	if !ok {
		return &ServicesExtraction{File: filePath, Functions: []WebServiceFunction{}}
	}

	functions := []WebServiceFunction{}
	for _, entry := range phparray.SplitKeyedEntries(body, functionKeyPattern) {
		classname := phparray.ExtractString(entry.Body, "classname")
		description := phparray.ExtractString(entry.Body, "description")
		if classname == "" && description == "" {
			continue
		}
		functions = append(functions, WebServiceFunction{
			Name:          entry.Key,
			ClassName:     classname,
			MethodName:    orDefault(phparray.ExtractString(entry.Body, "methodname"), "execute"),
			Description:   description,
			Type:          orDefault(phparray.ExtractString(entry.Body, "type"), "read"),
			Capabilities:  phparray.ExtractString(entry.Body, "capabilities"),
			Ajax:          phparray.ExtractBool(entry.Body, "ajax", false),
			LoginRequired: phparray.ExtractBool(entry.Body, "loginrequired", true),
		})
	}
	return &ServicesExtraction{File: filePath, Functions: functions}
}

// ExtractPluginServices parses `pluginPath`/db/services.php. It returns nil when that file cannot
// be read.
func ExtractPluginServices(pluginPath string) *ServicesExtraction {
	return ParseServicesPhp(filepath.Join(pluginPath, "db", "services.php"))
}

// GetFunctionNames returns the web service function names of `e`, sorted. It returns nil when `e`
// is nil, which is the case for a plugin without db/services.php.
func GetFunctionNames(e *ServicesExtraction) []string {
	if e == nil {
		return nil
	}
	names := make([]string, len(e.Functions))
	for i, f := range e.Functions {
		names[i] = f.Name
	}
	sort.Strings(names)
	return names
}
