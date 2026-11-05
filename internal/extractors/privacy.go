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
)

// PrivacyProvider describes a plugin's Privacy API provider class, classes/privacy/provider.php:
// whether the file exists, and which of the two metadata interfaces its code (comments excluded)
// refers to — \core_privacy\local\metadata\null_provider (the plugin stores no personal data) or
// \core_privacy\local\metadata\provider (it declares the personal data it stores).
type PrivacyProvider struct {
	File       string
	Exists     bool
	IsNull     bool
	IsMetadata bool
}

// Patterns that identify the privacy metadata interfaces, written in full or through a `use`
// import.
var (
	nullProviderPattern     = regexp.MustCompile(`\bnull_provider\b`)
	metadataProviderPattern = regexp.MustCompile(`metadata\\provider\b`)
)

// DetectPrivacyProvider reads `pluginPath`/classes/privacy/provider.php and reports which privacy
// metadata interface it implements. Exists is false when the file is missing or unreadable.
func DetectPrivacyProvider(pluginPath string) PrivacyProvider {
	p := PrivacyProvider{File: "classes/privacy/provider.php"}
	raw, err := readFileCapped(filepath.Join(pluginPath, "classes", "privacy", "provider.php"))
	if err != nil {
		return p
	}
	code := stripCommentsAndHeredocs(string(raw))
	p.Exists = true
	p.IsNull = nullProviderPattern.MatchString(code)
	p.IsMetadata = metadataProviderPattern.MatchString(code)
	return p
}
