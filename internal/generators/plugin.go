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

package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/genutil"
	"github.com/oito2/mcp-build82/internal/legacyhooks"
	"github.com/oito2/mcp-build82/internal/phparray"
)

// PreloadedPluginData holds the results of every extractor a plugin's 12 generators need, so
// GenerateAllForPlugin runs each extractor at most once instead of up to 6 times.
type PreloadedPluginData struct {
	Schema       *extractors.DbSchema
	Events       *extractors.EventsExtraction
	Tasks        *extractors.TasksExtraction
	Services     *extractors.ServicesExtraction
	Capabilities *extractors.CapabilitiesExtraction
	Upgrade      *extractors.UpgradeExtraction
	Classes      extractors.ClassesExtraction
	Hooks        extractors.HooksExtraction
	Subplugins   []extractors.Subplugin
	Settings     *extractors.SettingsExtraction
}

// --- buildDirectoryTree (used only by GeneratePluginStructure) ---------------

const maxTreeDepth = 2

type treeEntry struct {
	name  string
	isDir bool
}

func listTreeEntries(dir string) []treeEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []treeEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		out = append(out, treeEntry{name: name, isDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isDir != out[j].isDir {
			return out[i].isDir // dirs first
		}
		return out[i].name < out[j].name
	})
	return out
}

func buildDirectoryTree(root string, dir string, depth int, prefix string, b *strings.Builder) {
	if depth > maxTreeDepth {
		return
	}
	entries := listTreeEntries(dir)
	for i, e := range entries {
		last := i == len(entries)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		name := e.name
		if e.isDir {
			name += "/"
		}
		fmt.Fprintf(b, "%s%s%s\n", prefix, branch, name)
		if e.isDir && depth < maxTreeDepth {
			buildDirectoryTree(root, filepath.Join(dir, e.name), depth+1, nextPrefix, b)
		}
	}
}

var pluginStructureKeyFiles = []string{
	"version.php", "lib.php", "locallib.php", "settings.php",
	"db/install.xml", "db/access.php", "db/events.php", "db/tasks.php", "db/services.php", "db/upgrade.php",
}

func GeneratePluginStructure(info extractors.PluginInfo) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_STRUCTURE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Structure", fmt.Sprintf("Directory tree and key files for %s.", genutil.EscapeMdCell(info.Component))))

		b.WriteString("## Directory Tree\n\n```\n")
		fmt.Fprintf(&b, "%s/\n", info.Name)
		buildDirectoryTree(info.Path, info.Path, 0, "", &b)
		b.WriteString("```\n\n")

		b.WriteString("## Key Files\n\n| File | Present |\n|---|---|\n")
		for _, f := range pluginStructureKeyFiles {
			mark := ""
			if _, err := os.Stat(filepath.Join(info.Path, f)); err == nil {
				mark = "✔"
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", f, mark)
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 1. GeneratePluginContext -------------------------------------------------

func GeneratePluginContext(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_CONTEXT.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		schema, events, tasks, services, caps := preloadOrExtractCore(info.Path, preloaded)

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Context", fmt.Sprintf("Metadata and feature summary for %s.", genutil.EscapeMdCell(info.Component))))

		fmt.Fprintf(&b, "## Metadata\n\n| Field | Value |\n|---|---|\n"+
			"| Component | `%s` |\n| Type | %s |\n| Version | %s |\n| Requires | %s |\n"+
			"| Display name | %s |\n| Maturity | %s |\n| Path | %s |\n\n",
			genutil.EscapeMdCell(info.Component), genutil.EscapeMdCell(info.Type), genutil.EscapeMdCell(info.Version),
			genutil.EscapeMdCell(info.Requires), genutil.EscapeMdCell(info.DisplayName), genutil.EscapeMdCell(info.Maturity),
			genutil.RelativeOrOriginal(info.MoodlePath, info.Path))

		b.WriteString("## Features\n\n| Feature | Count |\n|---|---|\n")
		fmt.Fprintf(&b, "| Database tables | %d |\n", tableCount(schema))
		fmt.Fprintf(&b, "| Event observers | %d |\n", len(eventsOf(events)))
		fmt.Fprintf(&b, "| Scheduled tasks | %d |\n", len(tasksOf(tasks)))
		fmt.Fprintf(&b, "| Web services | %d |\n", len(servicesOf(services)))
		fmt.Fprintf(&b, "| Capabilities | %d |\n", len(capsOf(caps)))

		return genutil.Write(output, b.String()), nil
	})
}

func tableCount(schema *extractors.DbSchema) int {
	if schema == nil {
		return 0
	}
	return len(schema.Tables)
}

func preloadOrExtractCore(pluginPath string, preloaded *PreloadedPluginData) (
	*extractors.DbSchema, *extractors.EventsExtraction, *extractors.TasksExtraction,
	*extractors.ServicesExtraction, *extractors.CapabilitiesExtraction,
) {
	schema := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.DbSchema { return p.Schema },
		func() *extractors.DbSchema { return extractors.ExtractPluginSchema(pluginPath) })
	events := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.EventsExtraction { return p.Events },
		func() *extractors.EventsExtraction { return extractors.ExtractPluginEvents(pluginPath) })
	tasks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.TasksExtraction { return p.Tasks },
		func() *extractors.TasksExtraction { return extractors.ExtractPluginTasks(pluginPath) })
	services := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.ServicesExtraction { return p.Services },
		func() *extractors.ServicesExtraction { return extractors.ExtractPluginServices(pluginPath) })
	caps := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.CapabilitiesExtraction { return p.Capabilities },
		func() *extractors.CapabilitiesExtraction { return extractors.ExtractPluginCapabilities(pluginPath) })
	return schema, events, tasks, services, caps
}

// --- 3. GeneratePluginDbTables -------------------------------------------------

func GeneratePluginDbTables(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_DB_TABLES.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		schema := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.DbSchema { return p.Schema },
			func() *extractors.DbSchema { return extractors.ExtractPluginSchema(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Database Tables", fmt.Sprintf("Database schema for %s.", genutil.EscapeMdCell(info.Component))))
		var rows []string
		if schema != nil {
			for _, t := range schema.Tables {
				rows = append(rows, extractors.TableToMarkdown(t)+"\n")
			}
		}
		// No shared table header here — each table renders its own full markdown block (including
		// its own header) via extractors.TableToMarkdown, so header is empty.
		genutil.WriteTableOrPlaceholder(&b, "", rows, "no database tables")

		return genutil.Write(output, b.String()), nil
	})
}

// --- 4. GeneratePluginEvents ---------------------------------------------------

func GeneratePluginEvents(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_EVENTS.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		events := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.EventsExtraction { return p.Events },
			func() *extractors.EventsExtraction { return extractors.ExtractPluginEvents(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Events", fmt.Sprintf("Event observers registered by %s.", genutil.EscapeMdCell(info.Component))))
		var rows []string
		for _, o := range eventsOf(events) {
			rows = append(rows, fmt.Sprintf("| `%s` | `%s` | %d | %v |\n", genutil.EscapeMdCell(o.EventName), genutil.EscapeMdCell(o.Callback), o.Priority, o.Internal))
		}
		genutil.WriteTableOrPlaceholder(&b, "| Event | Callback | Priority | Internal |\n|---|---|---|---|\n", rows, "no events")

		return genutil.Write(output, b.String()), nil
	})
}

// --- 5. GeneratePluginDependencies (the busiest generator) --------------------

func GeneratePluginDependencies(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_DEPENDENCIES.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		tasks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.TasksExtraction { return p.Tasks },
			func() *extractors.TasksExtraction { return extractors.ExtractPluginTasks(info.Path) })
		services := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.ServicesExtraction { return p.Services },
			func() *extractors.ServicesExtraction { return extractors.ExtractPluginServices(info.Path) })
		caps := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.CapabilitiesExtraction { return p.Capabilities },
			func() *extractors.CapabilitiesExtraction { return extractors.ExtractPluginCapabilities(info.Path) })
		upgrade := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.UpgradeExtraction { return p.Upgrade },
			func() *extractors.UpgradeExtraction { return extractors.ExtractPluginUpgrade(info.Path) })
		hooks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.HooksExtraction { return p.Hooks },
			func() extractors.HooksExtraction {
				h, _ := extractors.ExtractPluginHooks(info.Path, info.Component)
				return h
			})
		subplugins := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) []extractors.Subplugin { return p.Subplugins },
			func() []extractors.Subplugin { return extractors.ExtractSubplugins(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Dependencies", fmt.Sprintf("Tasks, services, capabilities, hooks, and upgrade history for %s.", genutil.EscapeMdCell(info.Component))))

		b.WriteString("## Tasks\n\n")
		var taskRows []string
		if tasks != nil {
			for _, t := range tasks.Tasks {
				taskRows = append(taskRows, fmt.Sprintf("| `%s` | `%s` |\n", genutil.EscapeMdCell(t.ClassName), genutil.EscapeMdCell(extractors.FormatCronSchedule(t))))
			}
		}
		genutil.WriteTableOrPlaceholder(&b, "| Class | Schedule |\n|---|---|\n", taskRows, "no scheduled tasks")

		b.WriteString("\n## Services\n\n")
		var serviceRows []string
		if services != nil {
			for _, fn := range services.Functions {
				serviceRows = append(serviceRows, fmt.Sprintf("| `%s` | %s |\n", genutil.EscapeMdCell(fn.Name), genutil.EscapeMdCell(fn.Type)))
			}
		}
		genutil.WriteTableOrPlaceholder(&b, "| Function | Type |\n|---|---|\n", serviceRows, "no services")

		b.WriteString("\n## Capabilities\n\n")
		var capRows []string
		if caps != nil {
			for _, c := range caps.Capabilities {
				capRows = append(capRows, fmt.Sprintf("| `%s` | %s |\n", genutil.EscapeMdCell(c.Name), genutil.EscapeMdCell(c.CapType)))
			}
		}
		genutil.WriteTableOrPlaceholder(&b, "| Capability | Type |\n|---|---|\n", capRows, "no capabilities")

		b.WriteString("\n## Hook API\n\n")
		writeHookApiSection(&b, hooks)

		b.WriteString("\n## Upgrade History\n\n| Version | Description |\n|---|---|\n")
		if upgrade != nil {
			for _, s := range upgrade.Steps {
				fmt.Fprintf(&b, "| %s | %s |\n", genutil.EscapeMdCell(s.Version), genutil.EscapeMdCell(s.Description))
			}
		}

		b.WriteString("\n## Subplugins\n\n")
		if len(subplugins) == 0 {
			b.WriteString("_This plugin does not host subplugins of its own._\n")
		} else {
			b.WriteString("| Type | Path |\n|---|---|\n")
			for _, s := range subplugins {
				fmt.Fprintf(&b, "| `%s` | %s |\n", genutil.EscapeMdCell(s.Type), s.Path)
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// writeHookApiSection renders the Hook API sub-section shared by GeneratePluginDependencies and
// GeneratePluginCallbackIndex. Steps 1-3 (registered callbacks / definitions / "doesn't use hooks")
// and step 4 (legacy warnings) are independent conditionals, not mutually exclusive.
func writeHookApiSection(b *strings.Builder, hooks extractors.HooksExtraction) {
	if len(hooks.Callbacks) > 0 {
		b.WriteString("### Registered Callbacks (db/hooks.php)\n\n| Hook | Callback | Priority |\n|---|---|---|\n")
		for _, c := range hooks.Callbacks {
			fmt.Fprintf(b, "| `%s` | `%s` | %d |\n", genutil.EscapeMdCell(c.HookName), genutil.EscapeMdCell(c.Callback), c.Priority)
		}
		b.WriteString("\n")
	}
	if len(hooks.Definitions) > 0 {
		b.WriteString("### Hook Definitions (classes/hook/)\n\n| Class | Description | Tags |\n|---|---|---|\n")
		for _, d := range hooks.Definitions {
			fmt.Fprintf(b, "| `%s` | %s | %s |\n", genutil.EscapeMdCell(d.ClassName), genutil.EscapeMdCell(d.Description), genutil.EscapeMdCell(strings.Join(d.Tags, ", ")))
		}
		b.WriteString("\n")
	}
	if !extractors.PluginUsesHookApi(hooks) {
		b.WriteString("_This plugin does not use the Hook API._\n\n")
	}
	if len(hooks.LegacyWarnings) > 0 {
		b.WriteString("### ⚠ Legacy Callbacks — Migration Required\n\n| Legacy Function | Replaced By |\n|---|---|\n")
		for _, w := range hooks.LegacyWarnings {
			fmt.Fprintf(b, "| `%s` | `%s` |\n", genutil.EscapeMdCell(w.LegacyFunction), w.ReplacedBy)
		}
		b.WriteString("\n")
		for _, w := range hooks.LegacyWarnings {
			fmt.Fprintf(b, "- %s\n", genutil.EscapeMdCell(w.Guidance))
		}
	}
}

// --- 6. GeneratePluginFunctionIndex --------------------------------------------

func GeneratePluginFunctionIndex(info extractors.PluginInfo) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_FUNCTION_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Function Index", fmt.Sprintf("Every top-level function found in %s.", genutil.EscapeMdCell(info.Component))))

		files := globMoodleSuffix(info.Path, ".php")
		sort.Strings(files)
		for _, f := range files {
			fns := extractors.ExtractFunctionsFromPhpFile(f)
			if len(fns) == 0 {
				continue
			}
			rel, _ := filepath.Rel(info.Path, f)
			fmt.Fprintf(&b, "## %s\n\n", filepath.ToSlash(rel))
			for _, fn := range fns {
				fmt.Fprintf(&b, "- `%s()` (line %d)\n", genutil.EscapeMdCell(fn.Name), fn.Line)
			}
			b.WriteString("\n")
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 7. GeneratePluginCallbackIndex --------------------------------------------

// legacyCallbackSuffixesForIndex is derived from legacyhooks.Map's keys (sorted, for deterministic
// table ordering), so a new entry in legacy_hooks.json shows up here automatically.
var legacyCallbackSuffixesForIndex = sortedLegacyHookSuffixes()

func sortedLegacyHookSuffixes() []string {
	suffixes := make([]string, 0, len(legacyhooks.Map))
	for suffix := range legacyhooks.Map {
		suffixes = append(suffixes, suffix)
	}
	sort.Strings(suffixes)
	return suffixes
}

func GeneratePluginCallbackIndex(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_CALLBACK_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		hooks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.HooksExtraction { return p.Hooks },
			func() extractors.HooksExtraction {
				h, _ := extractors.ExtractPluginHooks(info.Path, info.Component)
				return h
			})

		legacyWithReplacement := map[string]bool{}
		for _, w := range hooks.LegacyWarnings {
			legacyWithReplacement[w.LegacyFunction] = true
		}

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Callback Index", fmt.Sprintf("Legacy and Hook API callbacks for %s.", genutil.EscapeMdCell(info.Component))))

		libContent, _ := os.ReadFile(filepath.Join(info.Path, "lib.php"))
		locallibContent, _ := os.ReadFile(filepath.Join(info.Path, "locallib.php"))

		b.WriteString("### Legacy lib.php Callbacks\n\n| Function | Hook Replacement Available |\n|---|---|\n")
		for _, suffix := range legacyCallbackSuffixesForIndex {
			fn := info.Component + "_" + suffix
			if functionExistsInContent(libContent, fn) || functionExistsInContent(locallibContent, fn) {
				mark := ""
				if legacyWithReplacement[fn] {
					mark = "⚠"
				}
				fmt.Fprintf(&b, "| `%s` | %s |\n", genutil.EscapeMdCell(fn), mark)
			}
		}
		b.WriteString("\n")

		writeHookApiSection(&b, hooks)

		return genutil.Write(output, b.String()), nil
	})
}

// functionExistsInContent reports whether name is declared as a PHP function in content. Anchored
// at the start of a line (allowing leading whitespace) and case-insensitive (PHP function names
// are case-insensitive), so it matches a space before the parenthesis (`function name ()`) and a
// differently-cased declaration. Note this does NOT distinguish a top-level function from a
// same-named method inside a class (never a valid legacy Moodle callback, which requires
// top-level) — a normally-indented class method still matches, since indentation alone can't be
// told apart from a top-level declaration at the line-regex level. Reusing
// extractors.ExtractFunctionsFromPhpFile (which does know real scope) would close that gap fully,
// at the cost of a full parse per call instead of a single regex match — left as a possible future
// improvement.
//
// This runs once per legacy callback suffix × file × plugin in GenerateAllForPluginCore/batch
// generation, so the pattern (keyed on name, the only variable part) is compiled once and cached
// via phparray.CachedPattern instead of recompiling on every call. Safe for concurrent use.
func functionExistsInContent(content []byte, name string) bool {
	if content == nil {
		return false
	}
	re := phparray.CachedPattern(`(?im)^\s*function\s+` + regexp.QuoteMeta(name) + `\s*\(`)
	return re.Match(content)
}

// --- 8. GeneratePluginEndpointIndex --------------------------------------------

func GeneratePluginEndpointIndex(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_ENDPOINT_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		services := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.ServicesExtraction { return p.Services },
			func() *extractors.ServicesExtraction { return extractors.ExtractPluginServices(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Endpoint Index", fmt.Sprintf("Web services, AJAX endpoints, and AMD modules in %s.", genutil.EscapeMdCell(info.Component))))

		b.WriteString("### Web Services\n\n")
		var serviceRows []string
		if services != nil {
			for _, fn := range services.Functions {
				serviceRows = append(serviceRows, fmt.Sprintf("| `%s` | `%s` | %v |\n", genutil.EscapeMdCell(fn.Name), genutil.EscapeMdCell(fn.ClassName), fn.Ajax))
			}
		}
		genutil.WriteTableOrPlaceholder(&b, "| Function | Class | AJAX |\n|---|---|---|\n", serviceRows, "no web services")

		b.WriteString("\n### AJAX Endpoints\n\n")
		// globMoodleBasename (not globMoodleSuffix): a file merely ending in "ajax.php"
		// (e.g. "notajax.php") is not the conventional AJAX entrypoint and must not be listed as one.
		ajaxFiles := globMoodleBasename(info.Path, "ajax.php")
		if len(ajaxFiles) == 0 {
			b.WriteString("_(none found)_\n")
		} else {
			sort.Strings(ajaxFiles)
			for _, f := range ajaxFiles {
				rel, _ := filepath.Rel(info.Path, f)
				fmt.Fprintf(&b, "- %s\n", filepath.ToSlash(rel))
			}
		}

		b.WriteString("\n### AMD Modules\n\n")
		amdFiles := globMoodleSuffix(filepath.Join(info.Path, "amd", "src"), ".js")
		if len(amdFiles) == 0 {
			b.WriteString("_(none found)_\n")
		} else {
			sort.Strings(amdFiles)
			for _, f := range amdFiles {
				rel, _ := filepath.Rel(info.Path, f)
				fmt.Fprintf(&b, "- %s\n", filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 9. GeneratePluginRuntimeFlow ----------------------------------------------

var runtimeFlowEntryPointFiles = []string{
	"index.php", "view.php", "edit.php", "lib.php", "settings.php", "externallib.php",
}
var runtimeFlowCoreLogicFiles = []string{
	"locallib.php", "classes/manager.php", "classes/helper.php",
}

func GeneratePluginRuntimeFlow(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_RUNTIME_FLOW.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		// This generator only ever renders events/tasks/services, never schema or capabilities —
		// preloadOrExtractCore always extracts all 5 (schema + caps included), so calling it here
		// when unpreloaded wastes a db/install.xml + db/access.php parse on every standalone call.
		// Extract only what's actually used instead; the preloaded branch still reuses the
		// already-computed data at zero extra cost.
		events := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.EventsExtraction { return p.Events },
			func() *extractors.EventsExtraction { return extractors.ExtractPluginEvents(info.Path) })
		tasks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.TasksExtraction { return p.Tasks },
			func() *extractors.TasksExtraction { return extractors.ExtractPluginTasks(info.Path) })
		services := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.ServicesExtraction { return p.Services },
			func() *extractors.ServicesExtraction { return extractors.ExtractPluginServices(info.Path) })
		classes := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.ClassesExtraction { return p.Classes },
			func() extractors.ClassesExtraction { return extractors.ExtractPluginClasses(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Runtime Flow", fmt.Sprintf("Entry points and core logic for %s.", genutil.EscapeMdCell(info.Component))))

		b.WriteString("## Entry Points\n\n| File | Present |\n|---|---|\n")
		for _, f := range runtimeFlowEntryPointFiles {
			writeFileChecklistRow(&b, info.Path, f)
		}

		b.WriteString("\n## Core Logic\n\n| File | Present |\n|---|---|\n")
		for _, f := range runtimeFlowCoreLogicFiles {
			writeFileChecklistRow(&b, info.Path, f)
		}

		fmt.Fprintf(&b, "\n## Classes (%d)\n\n", len(classes.Classes))
		for _, c := range classes.Classes {
			fmt.Fprintf(&b, "- `%s`\n", genutil.EscapeMdCell(c.FQN))
		}

		fmt.Fprintf(&b, "\n## Events (%d)\n\n", len(eventsOf(events)))
		for _, o := range eventsOf(events) {
			fmt.Fprintf(&b, "- `%s`\n", genutil.EscapeMdCell(o.EventName))
		}

		fmt.Fprintf(&b, "\n## Tasks (%d)\n\n", len(tasksOf(tasks)))
		for _, t := range tasksOf(tasks) {
			fmt.Fprintf(&b, "- `%s`\n", genutil.EscapeMdCell(t.ClassName))
		}

		fmt.Fprintf(&b, "\n## Services (%d)\n\n", len(servicesOf(services)))
		for _, fn := range servicesOf(services) {
			fmt.Fprintf(&b, "- `%s`\n", genutil.EscapeMdCell(fn.Name))
		}

		return genutil.Write(output, b.String()), nil
	})
}

func writeFileChecklistRow(b *strings.Builder, pluginPath, file string) {
	mark := ""
	if _, err := os.Stat(filepath.Join(pluginPath, file)); err == nil {
		mark = "✔"
	}
	fmt.Fprintf(b, "| `%s` | %s |\n", file, mark)
}

// --- 10. GeneratePluginArchitecture ---------------------------------------------

func GeneratePluginArchitecture(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_ARCHITECTURE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		classes := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.ClassesExtraction { return p.Classes },
			func() extractors.ClassesExtraction { return extractors.ExtractPluginClasses(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Architecture", fmt.Sprintf("Class structure for %s.", genutil.EscapeMdCell(info.Component))))

		fmt.Fprintf(&b, "## Overview\n\n| Metric | Count |\n|---|---|\n| Classes | %d |\n\n", len(classes.Classes))

		byDir := map[string][]extractors.PhpClass{}
		var dirs []string
		for _, c := range classes.Classes {
			dir := filepath.ToSlash(filepath.Dir(c.File))
			if _, ok := byDir[dir]; !ok {
				dirs = append(dirs, dir)
			}
			byDir[dir] = append(byDir[dir], c)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			fmt.Fprintf(&b, "### %s\n\n", dir)
			for _, c := range byDir[dir] {
				fmt.Fprintf(&b, "- `%s` (%s)\n", genutil.EscapeMdCell(c.Name), genutil.EscapeMdCell(string(c.Kind)))
			}
			b.WriteString("\n")
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 11. GeneratePluginSettings --------------------------------------------------

func GeneratePluginSettings(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_SETTINGS.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		settings := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.SettingsExtraction { return p.Settings },
			func() *extractors.SettingsExtraction { return extractors.ExtractPluginSettings(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin Settings", fmt.Sprintf("Admin settings declared by %s's settings.php.", genutil.EscapeMdCell(info.Component))))

		if settings == nil || len(settings.Settings) == 0 {
			b.WriteString("_This plugin has no settings.php, or it declares no admin_setting_* entries._\n")
		} else {
			b.WriteString("| Name | Type |\n|---|---|\n")
			for _, s := range settings.Settings {
				name := s.Name
				if name == "" {
					name = "_(computed, not a literal)_"
				} else {
					name = genutil.EscapeMdCell(name)
				}
				fmt.Fprintf(&b, "| `%s` | `%s` |\n", name, genutil.EscapeMdCell(s.Type))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 12. GeneratePluginAiContext (the combined/primary file) -------------------

func GeneratePluginAiContext(info extractors.PluginInfo, preloaded *PreloadedPluginData) GeneratorResult {
	output := PluginOutputPath(info.Path, "PLUGIN_AI_CONTEXT.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		schema, events, tasks, services, caps := preloadOrExtractCore(info.Path, preloaded)
		classes := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.ClassesExtraction { return p.Classes },
			func() extractors.ClassesExtraction { return extractors.ExtractPluginClasses(info.Path) })
		hooks := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) extractors.HooksExtraction { return p.Hooks },
			func() extractors.HooksExtraction {
				h, _ := extractors.ExtractPluginHooks(info.Path, info.Component)
				return h
			})
		subplugins := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) []extractors.Subplugin { return p.Subplugins },
			func() []extractors.Subplugin { return extractors.ExtractSubplugins(info.Path) })
		settings := genutil.PreloadOr(preloaded, func(p *PreloadedPluginData) *extractors.SettingsExtraction { return p.Settings },
			func() *extractors.SettingsExtraction { return extractors.ExtractPluginSettings(info.Path) })

		var b strings.Builder
		b.WriteString(genutil.Header("Plugin AI Context", fmt.Sprintf("Combined quick-reference context for %s.", genutil.EscapeMdCell(info.Component))))

		b.WriteString("## Quick Reference\n\n")
		for _, f := range PluginContextFiles {
			if f == "PLUGIN_AI_CONTEXT.md" {
				continue
			}
			fmt.Fprintf(&b, "- [%s](./%s)\n", f, f)
		}
		b.WriteString("\n")

		fmt.Fprintf(&b, "## Plugin\n\n`%s` (%s) — %s, version %s\n\n",
			genutil.EscapeMdCell(info.Component), genutil.EscapeMdCell(info.Type), genutil.EscapeMdCell(info.DisplayName), genutil.EscapeMdCell(info.Version))
		fmt.Fprintf(&b, "## Database\n\n%d table(s)\n\n", tableCount(schema))
		fmt.Fprintf(&b, "## Events\n\n%d observer(s)\n\n", len(eventsOf(events)))
		fmt.Fprintf(&b, "## Tasks\n\n%d scheduled task(s)\n\n", len(tasksOf(tasks)))
		fmt.Fprintf(&b, "## Web Services\n\n%d function(s)\n\n", len(servicesOf(services)))
		fmt.Fprintf(&b, "## Capabilities\n\n%d capability(ies)\n\n", len(capsOf(caps)))
		fmt.Fprintf(&b, "## Hook API\n\n%d callback(s), %d definition(s)\n\n", len(hooks.Callbacks), len(hooks.Definitions))
		fmt.Fprintf(&b, "## Classes\n\n%d class(es)\n", len(classes.Classes))
		if len(subplugins) > 0 {
			fmt.Fprintf(&b, "\n## Subplugins\n\n%d subplugin type(s)\n", len(subplugins))
		}
		if settings != nil && len(settings.Settings) > 0 {
			fmt.Fprintf(&b, "\n## Settings\n\n%d admin setting(s)\n", len(settings.Settings))
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- GenerateAllForPlugin orchestrator ------------------------------------------

type PluginGeneratorResult struct {
	Plugin string
	Files  []GeneratorResult
}

// GenerateAllForPlugin runs every per-plugin generator, respecting the mtime cache, and returns
// one GeneratorResult per output file. Called by generate_plugin_context, the watcher, and any
// single-plugin caller that needs the cache loaded and persisted around exactly this one call.
// markAsDev has no default in Go — every caller must pass it explicitly (audit call sites:
// update_indexes's include_plugins path needs markAsDev=true, which must be passed as an explicit
// `true`, not Go's zero-value `false`).
func GenerateAllForPlugin(pluginPath, moodlePath string, markAsDev bool, existingInfo *extractors.PluginInfo) PluginGeneratorResult {
	cache.Global.EnsureLoaded(moodlePath)
	defer func() {
		if err := cache.Global.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "[build82] warning: failed to persist cache:", err)
		}
	}()
	return GenerateAllForPluginCore(pluginPath, moodlePath, markAsDev, existingInfo)
}

// GenerateAllForPluginCore is GenerateAllForPlugin's body without the cache load/save bracket —
// batch orchestrators (plugin_batch, update_indexes's include_plugins path) call this directly
// inside their own loop over many plugins, wrapping the whole loop in a single EnsureLoaded/Save
// pair instead of one disk write per plugin.
func GenerateAllForPluginCore(pluginPath, moodlePath string, markAsDev bool, existingInfo *extractors.PluginInfo) PluginGeneratorResult {
	var info extractors.PluginInfo
	if existingInfo != nil {
		info = *existingInfo
	} else {
		info, _ = extractors.DetectPlugin(pluginPath)
	}
	info.MoodlePath = moodlePath
	info.Path = pluginPath

	logMigrationFailures(MigrateLegacyPluginFiles(pluginPath))

	sources := cache.GetPluginSourceFiles(pluginPath)

	var (
		classes    extractors.ClassesExtraction
		hooks      extractors.HooksExtraction
		schema     *extractors.DbSchema
		events     *extractors.EventsExtraction
		tasks      *extractors.TasksExtraction
		services   *extractors.ServicesExtraction
		caps       *extractors.CapabilitiesExtraction
		upgrade    *extractors.UpgradeExtraction
		subplugins []extractors.Subplugin
		settings   *extractors.SettingsExtraction
	)
	var wg sync.WaitGroup
	wg.Add(10)
	go func() { defer wg.Done(); classes = extractors.ExtractPluginClasses(pluginPath) }()
	go func() { defer wg.Done(); hooks, _ = extractors.ExtractPluginHooks(pluginPath, info.Component) }()
	go func() { defer wg.Done(); schema = extractors.ExtractPluginSchema(pluginPath) }()
	go func() { defer wg.Done(); events = extractors.ExtractPluginEvents(pluginPath) }()
	go func() { defer wg.Done(); tasks = extractors.ExtractPluginTasks(pluginPath) }()
	go func() { defer wg.Done(); services = extractors.ExtractPluginServices(pluginPath) }()
	go func() { defer wg.Done(); caps = extractors.ExtractPluginCapabilities(pluginPath) }()
	go func() { defer wg.Done(); upgrade = extractors.ExtractPluginUpgrade(pluginPath) }()
	go func() { defer wg.Done(); subplugins = extractors.ExtractSubplugins(pluginPath) }()
	go func() { defer wg.Done(); settings = extractors.ExtractPluginSettings(pluginPath) }()
	wg.Wait()

	preloaded := &PreloadedPluginData{
		Schema:       schema,
		Events:       events,
		Tasks:        tasks,
		Services:     services,
		Capabilities: caps,
		Upgrade:      upgrade,
		Classes:      classes,
		Hooks:        hooks,
		Subplugins:   subplugins,
		Settings:     settings,
	}

	var (
		results []GeneratorResult
		mu      sync.Mutex
	)

	jobs := []func(){
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_CONTEXT.md"), sources, func() GeneratorResult { return GeneratePluginContext(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_STRUCTURE.md"), sources, func() GeneratorResult { return GeneratePluginStructure(info) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_DB_TABLES.md"), sources, func() GeneratorResult { return GeneratePluginDbTables(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_EVENTS.md"), sources, func() GeneratorResult { return GeneratePluginEvents(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_DEPENDENCIES.md"), sources, func() GeneratorResult { return GeneratePluginDependencies(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_CALLBACK_INDEX.md"), sources, func() GeneratorResult { return GeneratePluginCallbackIndex(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_FUNCTION_INDEX.md"), sources, func() GeneratorResult { return GeneratePluginFunctionIndex(info) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_ENDPOINT_INDEX.md"), sources, func() GeneratorResult { return GeneratePluginEndpointIndex(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_RUNTIME_FLOW.md"), sources, func() GeneratorResult { return GeneratePluginRuntimeFlow(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_ARCHITECTURE.md"), sources, func() GeneratorResult { return GeneratePluginArchitecture(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_SETTINGS.md"), sources, func() GeneratorResult { return GeneratePluginSettings(info, preloaded) }),
		genutil.RunCached(&results, &mu, PluginOutputPath(pluginPath, "PLUGIN_AI_CONTEXT.md"), sources, func() GeneratorResult { return GeneratePluginAiContext(info, preloaded) }),
	}
	var jobsWg sync.WaitGroup
	jobsWg.Add(len(jobs))
	for _, j := range jobs {
		j := j
		go func() { defer jobsWg.Done(); j() }()
	}
	jobsWg.Wait()

	if markAsDev {
		// If this write fails (e.g. a read-only plugin directory), the plugin is not marked
		// .indevelopment, so tools that depend on that marker (plugin_batch mode=dev,
		// list_dev_plugins, doctor, the watcher) would not see it; the failure is logged to stderr.
		markerPath := PluginOutputPath(pluginPath, ".indevelopment")
		if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "[build82] warning: failed to create %s: %s\n", filepath.Dir(markerPath), err)
		} else if err := os.WriteFile(markerPath, []byte(genutil.Timestamp()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[build82] warning: failed to write %s: %s\n", markerPath, err)
		}
	}

	return PluginGeneratorResult{Plugin: info.Component, Files: results}
}
