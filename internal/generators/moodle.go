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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/genutil"
)

// --- 1. GenerateAiContext ---------------------------------------------------

var aiContextDirectoryPurpose = [][2]string{
	{"admin/", "Site administration pages and admin tools"},
	{"auth/", "Authentication plugins"},
	{"blocks/", "Sidebar/dashboard block plugins"},
	{"cache/", "Cache stores and cache-related infrastructure"},
	{"course/", "Course management and course format plugins"},
	{"enrol/", "Enrolment method plugins"},
	{"filter/", "Text filter plugins"},
	{"grade/", "Gradebook, grade import/export/report plugins"},
	{"lib/", "Core Moodle libraries (moodlelib.php, accesslib.php, ...)"},
	{"local/", "Local, site-specific plugins"},
	{"mod/", "Activity module plugins"},
	{"question/", "Question bank and question type plugins"},
	{"report/", "Site/course report plugins"},
	{"theme/", "Theme plugins"},
	{"user/", "User profile pages and profile field plugins"},
}

var aiContextKeyApis = []string{
	"accesslib.php", "moodlelib.php", "filelib.php", "weblib.php", "gradelib.php",
	"completionlib.php", "enrollib.php", "grouplib.php", "externallib.php",
}

var aiContextCodingGuidelines = []string{
	"Follow the [Moodle coding style](https://moodledev.io/general/development/policies/codingstyle).",
	"Access the database only through the `$DB` API (`get_records`, `execute`, ...), never raw SQL.",
	"Call `require_login()` (and `require_capability()`) before any page performs a protected action.",
	"Read request parameters via `required_param()`/`optional_param()`, never `$_GET`/`$_POST` directly.",
	"Render output through `$OUTPUT`, not raw HTML echoed straight into the response.",
	"Guard every capability-sensitive action with an explicit `has_capability()`/`require_capability()` check.",
	"Escape all dynamic output (`s()`, `format_string()`, `format_text()`) before rendering it.",
}

func GenerateAiContext(moodlePath, moodleVersion string, pluginDirs []string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "AI_CONTEXT.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle AI Context", "Overview of this Moodle installation for AI coding assistants."))

		// The installation root has no meaningful path relative to itself (genutil.RelativeOrOriginal
		// would just yield ".") — its own directory name is the least-leaky value that's still useful
		// context (distinguishes between multiple local Moodle installs) without exposing the full
		// absolute tree (and often an OS username baked into it) into a file meant to travel with the
		// repo.
		fmt.Fprintf(&b, "## Installation\n\n| Field | Value |\n|---|---|\n| Version | %s |\n| Path | %s |\n| Plugins found | %d |\n\n",
			moodleVersion, filepath.Base(moodlePath), len(pluginDirs))

		b.WriteString("## Directory Purpose\n\n| Directory | Purpose |\n|---|---|\n")
		for _, row := range aiContextDirectoryPurpose {
			fmt.Fprintf(&b, "| `%s` | %s |\n", row[0], row[1])
		}
		b.WriteString("\n")

		b.WriteString("## Key APIs (lib/)\n\n| File |\n|---|\n")
		for _, f := range aiContextKeyApis {
			fmt.Fprintf(&b, "| `%s` |\n", f)
		}
		b.WriteString("\n")

		b.WriteString("## Coding Guidelines\n\n")
		for _, g := range aiContextCodingGuidelines {
			fmt.Fprintf(&b, "- %s\n", g)
		}
		b.WriteString("\n")

		b.WriteString("## Index Files\n\n")
		for _, f := range GlobalContextFilenames {
			if f == "AI_CONTEXT.md" {
				continue
			}
			fmt.Fprintf(&b, "- [%s](./%s)\n", f, f)
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 2. GenerateApiIndex -----------------------------------------------------

// apiFunctionLine renders one function line. This exact format is load-bearing: search_api's
// visibility filter substring-matches "@deprecated" in the rendered line — keep both in sync.
func apiFunctionLine(f extractors.ApiFunction) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- `%s()`", genutil.EscapeMdCell(f.Name))
	if f.Visibility == extractors.VisDeprecated {
		if f.Doc != nil && f.Doc.Deprecated != "" && f.Doc.Deprecated != "yes" {
			fmt.Fprintf(&b, " ~~**@deprecated**: %s~~", genutil.EscapeMdCell(f.Doc.Deprecated))
		} else {
			b.WriteString(" ~~**@deprecated**~~")
		}
	}
	if f.Doc != nil && f.Doc.Summary != "" {
		fmt.Fprintf(&b, " — %s", genutil.EscapeMdCell(f.Doc.Summary))
	}
	if f.Doc != nil && f.Doc.Returns != "" {
		fmt.Fprintf(&b, " → `%s`", genutil.EscapeMdCell(f.Doc.Returns))
	}
	if f.Doc != nil && f.Doc.Since != "" {
		fmt.Fprintf(&b, " _(since %s)_", genutil.EscapeMdCell(f.Doc.Since))
	}
	return b.String()
}

func GenerateApiIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		extraction := extractors.ExtractMoodleApi(moodlePath)

		var b strings.Builder
		b.WriteString(genutil.Header("Moodle API Index", "Public and deprecated functions in lib/, grouped by source file."))

		c := extraction.Counts
		fmt.Fprintf(&b, "## Summary\n\n| Visibility | Count |\n|---|---|\n| Public | %d |\n| Deprecated | %d |\n"+
			"| Internal (excluded) | %d |\n| Private (excluded) | %d |\n| Unverified (excluded) | %d |\n\n",
			c.Public, c.Deprecated, c.Internal, c.Private, c.Unverified)

		byFile := map[string][]extractors.ApiFunction{}
		var files []string
		for _, f := range extraction.Functions {
			if _, ok := byFile[f.File]; !ok {
				files = append(files, f.File)
			}
			byFile[f.File] = append(byFile[f.File], f)
		}
		for _, file := range files {
			fmt.Fprintf(&b, "## %s\n\n", file)
			for _, fn := range byFile[file] {
				b.WriteString(apiFunctionLine(fn))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 3. GenerateEventsIndex ---------------------------------------------------

func GenerateEventsIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_EVENTS_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		type row struct{ event, callback, source string }
		var rows []row
		for _, f := range globMoodleSuffix(moodlePath, "db/events.php") {
			extraction := extractors.ParseEventsPhp(f)
			if extraction == nil {
				continue
			}
			rel, _ := filepath.Rel(moodlePath, f)
			for _, o := range extraction.Observers {
				rows = append(rows, row{o.EventName, o.Callback, filepath.ToSlash(rel)})
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].event < rows[j].event })

		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Events Index", "Every event observer registered across all plugins."))
		b.WriteString("| Event | Callback | Source |\n|---|---|---|\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", genutil.EscapeMdCell(r.event), genutil.EscapeMdCell(r.callback), r.source)
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 4. GenerateTasksIndex ----------------------------------------------------

func GenerateTasksIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_TASKS_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Tasks Index", "Every scheduled task registered across all plugins."))
		b.WriteString("| Class | Schedule | Blocking | Source |\n|---|---|---|---|\n")

		// Not sorted — file iteration (glob) order, deliberately.
		for _, f := range globMoodleSuffix(moodlePath, "db/tasks.php") {
			extraction := extractors.ParseTasksPhp(f)
			if extraction == nil {
				continue
			}
			rel, _ := filepath.Rel(moodlePath, f)
			for _, task := range extraction.Tasks {
				fmt.Fprintf(&b, "| `%s` | `%s` | %v | %s |\n",
					genutil.EscapeMdCell(task.ClassName), genutil.EscapeMdCell(extractors.FormatCronSchedule(task)), task.Blocking, filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 5. GenerateServicesIndex -------------------------------------------------

func GenerateServicesIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_SERVICES_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Services Index", "Every web service function registered across all plugins."))
		b.WriteString("| Function | Type | AJAX | Class | Source |\n|---|---|---|---|---|\n")

		for _, f := range globMoodleSuffix(moodlePath, "db/services.php") {
			extraction := extractors.ParseServicesPhp(f)
			if extraction == nil {
				continue
			}
			rel, _ := filepath.Rel(moodlePath, f)
			for _, fn := range extraction.Functions {
				fmt.Fprintf(&b, "| `%s` | %s | %v | `%s` | %s |\n",
					genutil.EscapeMdCell(fn.Name), genutil.EscapeMdCell(fn.Type), fn.Ajax, genutil.EscapeMdCell(fn.ClassName), filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 6. GenerateDbTablesIndex -------------------------------------------------

func GenerateDbTablesIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_DB_TABLES_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle DB Tables Index", "Every database table declared across all plugins."))
		b.WriteString("| Table | Fields | Source |\n|---|---|---|\n")

		for _, f := range globMoodleSuffix(moodlePath, "db/install.xml") {
			schema := extractors.ParseInstallXml(f)
			if schema == nil {
				continue
			}
			rel, _ := filepath.Rel(moodlePath, f)
			for _, t := range schema.Tables {
				fmt.Fprintf(&b, "| `%s` | %d | %s |\n", genutil.EscapeMdCell(t.Name), len(t.Fields), filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 7. GenerateClassesIndex --------------------------------------------------

func GenerateClassesIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_CLASSES_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Classes Index", "Every class/interface/trait/enum found under a classes/ directory."))
		b.WriteString("| FQN | Kind | File |\n|---|---|---|\n")

		// Restricted glob: **/classes/**/*.php, not a full-installation scan.
		files := globMoodleClassesPhp(moodlePath)
		extraction := extractors.ExtractClassesFromFiles(files, moodlePath)
		for _, c := range extraction.Classes {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", genutil.EscapeMdCell(c.FQN), genutil.EscapeMdCell(string(c.Kind)), c.File)
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 8. GenerateCapabilitiesIndex ---------------------------------------------

func GenerateCapabilitiesIndex(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_CAPABILITIES_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Capabilities Index", "Every capability declared across all plugins."))
		b.WriteString("| Capability | Type | Context | Source |\n|---|---|---|---|\n")

		for _, f := range globMoodleSuffix(moodlePath, "db/access.php") {
			extraction := extractors.ParseAccessPhp(f)
			if extraction == nil {
				continue
			}
			rel, _ := filepath.Rel(moodlePath, f)
			for _, c := range extraction.Capabilities {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n",
					genutil.EscapeMdCell(c.Name), genutil.EscapeMdCell(c.CapType), genutil.EscapeMdCell(c.ContextLevel), filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 9. GeneratePluginIndex ---------------------------------------------------

func GeneratePluginIndex(moodlePath string, pluginDirs []string, infoCache *pluginInfoCache) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_PLUGIN_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		dirs := append([]string{}, pluginDirs...)
		sort.Strings(dirs)

		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Plugin Index", "Every plugin found in this installation."))
		b.WriteString("| Component | Type | Name | Version | Path |\n|---|---|---|---|---|\n")
		for _, d := range dirs {
			info := infoCache.get(d)
			rel, _ := filepath.Rel(moodlePath, d)
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n",
				genutil.EscapeMdCell(info.Component), genutil.EscapeMdCell(info.Type), genutil.EscapeMdCell(info.Name),
				genutil.EscapeMdCell(info.Version), filepath.ToSlash(rel))
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 10. GenerateDevRules (fully static) -------------------------------------

func GenerateDevRules(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_DEV_RULES.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		content := genutil.Header("Moodle Development Rules", "Coding-standard reference snippets for this installation.") + `
## Security

` + "```php" + `
require_login($courseid);
require_capability('local/demo:view', $context);
$id = required_param('id', PARAM_INT);
echo format_string($record->name);
` + "```" + `

## Database API

` + "```php" + `
global $DB;
$records = $DB->get_records('local_demo_records', ['id' => $id]);
` + "```" + `

## Plugin File Structure

` + "```" + `
local/demo/
├── version.php
├── lib.php
├── db/
│   ├── access.php
│   ├── events.php
│   ├── tasks.php
│   └── install.xml
└── classes/
    ├── task/
    └── hook/
` + "```" + `

## Triggering an Event

` + "```php" + `
$event = \local_demo\event\course_viewed::create(['context' => $context]);
$event->trigger();
` + "```" + `

## Scheduled Task Class

` + "```php" + `
namespace local_demo\task;

class send_reminders extends \core\task\scheduled_task {
    public function get_name() {
        return get_string('sendreminders', 'local_demo');
    }
    public function execute() {
        // ...
    }
}
` + "```" + `
`
		return genutil.Write(output, content), nil
	})
}

// --- 11. GeneratePluginGuide (static + moodleVersion) ------------------------

var pluginGuideComponentNaming = [][2]string{
	{"mod_*", "Activity modules (mod/{name})"},
	{"block_*", "Blocks (blocks/{name})"},
	{"local_*", "Local plugins (local/{name})"},
	{"auth_*", "Authentication (auth/{name})"},
	{"enrol_*", "Enrolment (enrol/{name})"},
	{"theme_*", "Themes (theme/{name})"},
	{"report_*", "Reports (report/{name})"},
	{"filter_*", "Text filters (filter/{name})"},
	{"tool_*", "Admin tools (admin/tool/{name})"},
}

func GeneratePluginGuide(moodlePath, moodleVersion string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_PLUGIN_GUIDE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle Plugin Guide", fmt.Sprintf("How to scaffold a new plugin for Moodle %s.", moodleVersion)))

		b.WriteString("## Component Naming\n\n| Prefix | Meaning |\n|---|---|\n")
		for _, row := range pluginGuideComponentNaming {
			fmt.Fprintf(&b, "| `%s` | %s |\n", row[0], row[1])
		}

		b.WriteString("\n## Required Files\n\n| File | Purpose |\n|---|---|\n" +
			"| `version.php` | Plugin identity, version, dependencies |\n" +
			"| `lib.php` | Legacy callback hooks |\n" +
			"| `db/install.xml` | Database schema |\n" +
			"| `lang/en/{component}.php` | English language strings |\n\n")

		fmt.Fprintf(&b, "## version.php Template\n\n```php\n<?php\ndefined('MOODLE_INTERNAL') || die();\n\n"+
			"$plugin->component = 'local_yourname';\n$plugin->version   = %s;\n$plugin->requires  = %s;\n"+
			"$plugin->maturity  = MATURITY_STABLE;\n$plugin->release   = '1.0.0';\n```\n\n",
			moodleVersion, moodleVersion)

		b.WriteString("## Autoloaded Class Paths\n\n" +
			"- `classes/task/my_task.php` → `\\local_yourname\\task\\my_task`\n" +
			"- `classes/event/my_event.php` → `\\local_yourname\\event\\my_event`\n" +
			"- `classes/hook/my_hook.php` → `\\local_yourname\\hook\\my_hook`\n")

		return genutil.Write(output, b.String()), nil
	})
}

// --- 12. GenerateAiWorkspace --------------------------------------------------

func GenerateAiWorkspace(moodlePath string, pluginDirs []string, infoCache *pluginInfoCache) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_AI_WORKSPACE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		devDirs := FindDevPlugins(moodlePath)
		sort.Strings(devDirs)

		var withContext []string
		for _, d := range pluginDirs {
			if _, err := os.Stat(PluginOutputPath(d, "PLUGIN_AI_CONTEXT.md")); err == nil {
				withContext = append(withContext, d)
			}
		}
		sort.Strings(withContext)

		var b strings.Builder
		b.WriteString(genutil.Header("Moodle AI Workspace", "Current development status of this installation."))
		fmt.Fprintf(&b, "## Summary\n\n| Metric | Count |\n|---|---|\n| Total plugins | %d |\n"+
			"| In development | %d |\n| With AI context | %d |\n\n", len(pluginDirs), len(devDirs), len(withContext))

		b.WriteString("## How to Use\n\n" +
			"1. Run `init_moodle_context` once per installation to generate the global index files.\n" +
			"2. Run `generate_plugin_context` on the plugin you're about to work on.\n" +
			"3. Read `PLUGIN_AI_CONTEXT.md` first — it links to every other generated file for that plugin.\n" +
			"4. Use `search_plugins`/`search_api` to find existing code before writing new code.\n" +
			"5. Re-run `update_indexes` after making structural changes (new events, tasks, capabilities).\n\n")

		if len(devDirs) > 0 {
			b.WriteString("## Plugins In Development\n\n| Component | Path |\n|---|---|\n")
			for _, d := range devDirs {
				info := infoCache.get(d)
				rel, _ := filepath.Rel(moodlePath, d)
				fmt.Fprintf(&b, "| `%s` | %s |\n", genutil.EscapeMdCell(info.Component), filepath.ToSlash(rel))
			}
			b.WriteString("\n")
		}

		if len(withContext) > 0 {
			b.WriteString("## Plugins With AI Context\n\n| Component | Path |\n|---|---|\n")
			for _, d := range withContext {
				info := infoCache.get(d)
				rel, _ := filepath.Rel(moodlePath, d)
				fmt.Fprintf(&b, "| `%s` | %s |\n", genutil.EscapeMdCell(info.Component), filepath.ToSlash(rel))
			}
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- 13. GenerateAiIndex ------------------------------------------------------

// GenerateAiIndex is the public entry point for callers outside GenerateAll (the watcher package,
// and any future single-plugin-update tool) that don't already have pluginDirs/infoCache computed —
// it derives them fresh. GenerateAll itself calls the internal generateAiIndex directly with the
// values it already computed, to avoid a redundant FindPluginDirs glob.
func GenerateAiIndex(moodlePath, moodleVersion string) GeneratorResult {
	return generateAiIndex(moodlePath, moodleVersion, FindPluginDirs(moodlePath), newPluginInfoCache())
}

func generateAiIndex(moodlePath, moodleVersion string, pluginDirs []string, infoCache *pluginInfoCache) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MOODLE_AI_INDEX.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		var b strings.Builder
		b.WriteString(genutil.Header("Moodle AI Index", "Links to every generated context file that currently exists."))

		b.WriteString("## Global Files\n\n")
		for _, f := range GlobalContextFilenames {
			if f == "MOODLE_AI_INDEX.md" {
				continue
			}
			if _, err := os.Stat(GlobalOutputPath(moodlePath, f)); err == nil {
				fmt.Fprintf(&b, "- [%s](./%s)\n", f, f)
			}
		}

		b.WriteString("\n## Plugins With AI Context\n\n")
		dirs := append([]string{}, pluginDirs...)
		sort.Strings(dirs)
		for _, d := range dirs {
			contextFile := PluginOutputPath(d, "PLUGIN_AI_CONTEXT.md")
			if _, err := os.Stat(contextFile); err != nil {
				continue
			}
			info := infoCache.get(d)
			rel, _ := filepath.Rel(moodlePath, contextFile)
			fmt.Fprintf(&b, "- [%s](./%s)\n", genutil.EscapeMdCell(info.Component), filepath.ToSlash(rel))
		}

		return genutil.Write(output, b.String()), nil
	})
}

// --- GenerateCtags -------------------------------------------------------------

// GenerateCtags shells out to ctags. Skipped gracefully (not a failure) if ctags isn't on PATH.
func GenerateCtags(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "tags")
	if _, err := exec.LookPath("ctags"); err != nil {
		return GeneratorResult{File: output, Success: true, Skipped: true}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return GeneratorResult{File: output, Error: err.Error()}
	}
	// "--" before moodlePath prevents a path beginning with "-" (e.g. a misconfigured
	// BUILD82_MOODLE_PATH) from being interpreted as a ctags flag instead of the target directory.
	// exec.Command never invokes a shell, so this isn't
	// shell-injection risk — just defensive argument-boundary hardening.
	cmd := exec.Command("ctags", "-R", "--languages=PHP", "--exclude=vendor", "--exclude=node_modules", "-f", output, "--", moodlePath)
	if err := cmd.Run(); err != nil {
		return GeneratorResult{File: output, Error: err.Error()}
	}
	cache.Global.Mark(output)
	return GeneratorResult{File: output, Success: true}
}

// --- GenerateAll orchestrator --------------------------------------------------

// GenerateAll runs every global generator, respecting the mtime cache, and returns one
// GeneratorResult per output file. Called by init_moodle_context and update_indexes.
func GenerateAll(moodlePath, moodleVersion string) []GeneratorResult {
	var (
		results []GeneratorResult
		mu      sync.Mutex
	)

	cache.Global.EnsureLoaded(moodlePath)
	defer func() {
		if err := cache.Global.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "[build82] warning: failed to persist cache:", err)
		}
	}()

	logMigrationFailures(MigrateLegacyGlobalFiles(moodlePath))

	globalSources := cache.GetMoodleSourceFiles(moodlePath)
	pluginDirs := FindPluginDirs(moodlePath)
	infoCache := newPluginInfoCache()

	var eventsSrc, tasksSrc, servicesSrc, dbTablesSrc, capsSrc []string
	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); eventsSrc = globMoodleSuffix(moodlePath, "db/events.php") }()
	go func() { defer wg.Done(); tasksSrc = globMoodleSuffix(moodlePath, "db/tasks.php") }()
	go func() { defer wg.Done(); servicesSrc = globMoodleSuffix(moodlePath, "db/services.php") }()
	go func() { defer wg.Done(); dbTablesSrc = globMoodleSuffix(moodlePath, "db/install.xml") }()
	go func() { defer wg.Done(); capsSrc = globMoodleSuffix(moodlePath, "db/access.php") }()
	wg.Wait()

	pluginVersionFiles := make([]string, len(pluginDirs))
	for i, d := range pluginDirs {
		pluginVersionFiles[i] = filepath.Join(d, "version.php")
	}

	tasks := []func(){
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "AI_CONTEXT.md"), pluginVersionFiles,
			func() GeneratorResult { return GenerateAiContext(moodlePath, moodleVersion, pluginDirs) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md"), globalSources,
			func() GeneratorResult { return GenerateApiIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_EVENTS_INDEX.md"), eventsSrc,
			func() GeneratorResult { return GenerateEventsIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_TASKS_INDEX.md"), tasksSrc,
			func() GeneratorResult { return GenerateTasksIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_SERVICES_INDEX.md"), servicesSrc,
			func() GeneratorResult { return GenerateServicesIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_DB_TABLES_INDEX.md"), dbTablesSrc,
			func() GeneratorResult { return GenerateDbTablesIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_CLASSES_INDEX.md"), globalSources,
			func() GeneratorResult { return GenerateClassesIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_CAPABILITIES_INDEX.md"), capsSrc,
			func() GeneratorResult { return GenerateCapabilitiesIndex(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_PLUGIN_INDEX.md"), pluginVersionFiles,
			func() GeneratorResult { return GeneratePluginIndex(moodlePath, pluginDirs, infoCache) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_DEV_RULES.md"), globalSources,
			func() GeneratorResult { return GenerateDevRules(moodlePath) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_PLUGIN_GUIDE.md"), globalSources,
			func() GeneratorResult { return GeneratePluginGuide(moodlePath, moodleVersion) }),
		genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_AI_WORKSPACE.md"), pluginVersionFiles,
			func() GeneratorResult { return GenerateAiWorkspace(moodlePath, pluginDirs, infoCache) }),
	}

	var wave1 sync.WaitGroup
	wave1.Add(len(tasks))
	for _, t := range tasks {
		t := t
		go func() { defer wave1.Done(); t() }()
	}
	wave1.Wait()

	// Wave 2: GenerateAiIndex must run after wave 1, since it checks disk existence of the files
	// wave 1 just wrote.
	genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "MOODLE_AI_INDEX.md"), pluginVersionFiles,
		func() GeneratorResult { return generateAiIndex(moodlePath, moodleVersion, pluginDirs, infoCache) })()

	genutil.RunCached(&results, &mu, GlobalOutputPath(moodlePath, "tags"), globalSources,
		func() GeneratorResult { return GenerateCtags(moodlePath) })()

	return results
}
