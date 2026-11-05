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

package resources

import (
	"regexp"
	"strings"
)

// pluginsWithContextURI is the URI of the aggregate resource listing the plugins with AI context.
const pluginsWithContextURI = "moodle://plugins/with-context"

// pluginURIPrefix starts every per-plugin resource URI: moodle://plugin/{component}{suffix}.
const pluginURIPrefix = "moodle://plugin/"

// componentPattern matches a Frankenstyle component as it appears in a per-plugin resource URI.
var componentPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// KnownURI reports whether `uri` names a resource this server serves: one of the global
// resources, the moodle://plugins/with-context aggregate, or a per-plugin resource
// moodle://plugin/{component}{suffix} for a well-formed component and one of the per-plugin
// suffixes. The plugin itself need not exist yet, so a client can subscribe before its files are
// generated.
func KnownURI(uri string) bool {
	if uri == pluginsWithContextURI {
		return true
	}
	for _, def := range globalResources {
		if uri == def.URI {
			return true
		}
	}
	rest, ok := strings.CutPrefix(uri, pluginURIPrefix)
	if !ok {
		return false
	}
	for _, def := range pluginFileResources {
		component, ok := strings.CutSuffix(rest, def.Suffix)
		if ok && componentPattern.MatchString(component) {
			return true
		}
	}
	return false
}

// PluginURIs returns the URIs of every per-plugin resource of `component`, in the order of
// pluginFileResources.
func PluginURIs(component string) []string {
	uris := make([]string, 0, len(pluginFileResources))
	for _, def := range pluginFileResources {
		uris = append(uris, pluginURIPrefix+component+def.Suffix)
	}
	return uris
}

// PluginIndexURIs returns the URIs of the global resources a per-plugin generation also changes:
// the AI index (moodle://index), which lists every plugin's AI context, and the
// moodle://plugins/with-context aggregate.
func PluginIndexURIs() []string {
	return []string{"moodle://index", pluginsWithContextURI}
}

// GlobalURIs returns the URIs of every global resource plus the moodle://plugins/with-context
// aggregate, which a regeneration of the global index files changes.
func GlobalURIs() []string {
	uris := make([]string, 0, len(globalResources)+1)
	for _, def := range globalResources {
		uris = append(uris, def.URI)
	}
	return append(uris, pluginsWithContextURI)
}
