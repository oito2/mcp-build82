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
	"strings"
)

// supportedAssignPattern matches a $plugin->supported assignment whose value is a short or long
// array literal; group 1 is the value.
var supportedAssignPattern = regexp.MustCompile(`\$plugin\s*->\s*supported\s*=\s*(\[[^\]]*\]|array\s*\([^)]*\))`)

// ReadSupported returns the $plugin->supported array literal of `pluginPath`/version.php (outside
// comments) with its whitespace collapsed and no space inside the brackets, such as "[401, 405]",
// or "" when the file cannot be read
// or declares none.
func ReadSupported(pluginPath string) string {
	content, err := readFileCapped(filepath.Join(pluginPath, "version.php"))
	if err != nil {
		return ""
	}
	m := supportedAssignPattern.FindStringSubmatch(stripCommentsAndHeredocs(string(content)))
	if m == nil {
		return ""
	}
	v := strings.Join(strings.Fields(m[1]), " ")
	return strings.NewReplacer("[ ", "[", " ]", "]", "( ", "(", " )", ")").Replace(v)
}
