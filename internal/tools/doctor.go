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

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/phpcs"
)

// checkStatus is the outcome of one doctor check.
type checkStatus string

// Possible values of checkStatus, from best to worst.
const (
	statusOK   checkStatus = "ok"
	statusWarn checkStatus = "warn"
	statusFail checkStatus = "fail"
)

// checkResult is one doctor check: a label, its status, and an optional human-readable detail.
type checkResult struct {
	Label  string      `json:"label" jsonschema:"What was checked"`
	Status checkStatus `json:"status" jsonschema:"Result of the check: ok, warn or fail"`
	Detail string      `json:"detail,omitempty" jsonschema:"Human-readable detail of the result"`
}

// formatCheck renders `c` as one indented report line: status icon, label and optional detail.
func formatCheck(c checkResult) string {
	icon := map[checkStatus]string{statusOK: "✔", statusWarn: "⚠", statusFail: "✖"}[c.Status]
	detail := ""
	if c.Detail != "" {
		detail = " — " + c.Detail
	}
	return fmt.Sprintf("  %s %s%s", icon, c.Label, detail)
}

// staleThresholdDays is the age in days beyond which a generated file is reported as stale.
const staleThresholdDays = 7

// expectedGlobalFiles lists the global index files the doctor expects to exist.
var expectedGlobalFiles = generators.GlobalContextFilenames

// DoctorInput is the input of the doctor tool.
type DoctorInput struct {
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// DoctorPluginChecks groups the index-freshness checks of one development plugin.
type DoctorPluginChecks struct {
	Component string        `json:"component" jsonschema:"Frankenstyle component of the development plugin"`
	Checks    []checkResult `json:"checks" jsonschema:"Freshness checks of the plugin's generated context files"`
}

// DoctorCache reports the in-process cache counters and the on-disk cache file, if any.
type DoctorCache struct {
	Hits      int    `json:"hits" jsonschema:"Cache hits since the server started"`
	Misses    int    `json:"misses" jsonschema:"Cache misses since the server started"`
	Skips     int    `json:"skips" jsonschema:"Generations skipped because their output was fresh"`
	File      string `json:"file,omitempty" jsonschema:"Absolute path of the on-disk cache file, when it exists"`
	FileBytes int64  `json:"file_bytes,omitempty" jsonschema:"Size of the on-disk cache file in bytes"`
}

// DoctorOutput is the structured form of the doctor report: one field per report section, plus
// an overall verdict ("ok", "warn" or "fail"). Sections that could not run because the
// configuration is missing or unresolvable are left empty.
type DoctorOutput struct {
	SystemDependencies     []checkResult        `json:"system_dependencies" jsonschema:"Optional external tools (php, ctags, git, phpcs)"`
	Configuration          []checkResult        `json:"configuration" jsonschema:"build82 configuration checks"`
	MoodleInstallation     []checkResult        `json:"moodle_installation" jsonschema:"Checks of the configured Moodle installation"`
	GlobalIndexFiles       []checkResult        `json:"global_index_files" jsonschema:"Freshness of the global index files"`
	DevelopmentPlugins     []DoctorPluginChecks `json:"development_plugins" jsonschema:"Per .indevelopment plugin, the freshness of its context files"`
	LegacyFiles            []checkResult        `json:"legacy_files" jsonschema:"Generated files still outside .build82/ that a generation would migrate"`
	CrossPluginConsistency []checkResult        `json:"cross_plugin_consistency" jsonschema:"Consistency checks across the development plugins"`
	DeprecatedApiUsage     []checkResult        `json:"deprecated_api_usage" jsonschema:"Calls to deprecated core functions found in the development plugins"`
	CapabilityUsage        []checkResult        `json:"capability_usage" jsonschema:"Consistency between the capabilities each plugin defines and checks"`
	LangStringUsage        []checkResult        `json:"lang_string_usage" jsonschema:"Consistency between the language strings each plugin defines and uses"`
	UpgradeConsistency     []checkResult        `json:"upgrade_consistency" jsonschema:"Consistency between each plugin's db/upgrade.php steps, db/install.xml and version.php"`
	PrivacyApi             []checkResult        `json:"privacy_api" jsonschema:"Whether each plugin's Privacy API provider exists and matches the user data its tables store"`
	Cache                  *DoctorCache         `json:"cache,omitempty" jsonschema:"Cache counters and on-disk cache file; omitted when the report stopped early"`
	Verdict                checkStatus          `json:"verdict" jsonschema:"Worst status found across the check sections: ok, warn or fail"`
	Hint                   string               `json:"hint,omitempty" jsonschema:"What to do next when the report stopped early"`
}

// RegisterDoctorTool registers the doctor tool on `server`.
func RegisterDoctorTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "doctor",
		InputSchema: inputSchemaFor[DoctorInput](),
		Annotations: toolAnnotations("Doctor", true, false, true, false),
		Description: "Runs a full environment diagnostic: system dependencies, configuration, Moodle " +
			"installation, generated index freshness, dev plugins, legacy files pending migration, " +
			"cross-plugin consistency, deprecated core API usage, own-capability check consistency, " +
			"own-lang-string usage consistency, db/upgrade.php consistency with db/install.xml and version.php, " +
			"Privacy API provider consistency, and cache stats.",
	}, withRecover(handleDoctor))
}

// handleDoctor runs the diagnostic sections (system dependencies, configuration, Moodle
// installation, index freshness, dev plugins and their consistency checks, cache) and renders them
// as Markdown or, with `in.Format` set to JSON, as a DoctorOutput, which is the structured output
// in either format. It stops early when the
// configuration cannot be resolved (error result) or does not exist (reported as a failed check
// with a hint). The overall verdict is the worst status found. The error return is always nil:
// even a failed diagnosis carries its structured report.
func handleDoctor(ctx context.Context, req *mcp.CallToolRequest, in DoctorInput) (*mcp.CallToolResult, DoctorOutput, error) {
	var b strings.Builder
	out := DoctorOutput{}
	finish := func(isError bool) (*mcp.CallToolResult, DoctorOutput, error) {
		out.normalize()
		return structuredResult(in.Format, isError, b.String(), out)
	}
	b.WriteString("# build82 Doctor\n\n")

	// 1. System dependencies (concurrent).
	deps := checkSystemDependencies()
	out.SystemDependencies = deps
	b.WriteString("## System Dependencies\n\n")
	for _, c := range deps {
		fmt.Fprintln(&b, formatCheck(c))
	}
	b.WriteString("\n")

	// 2. Configuration.
	cfg, cfgErr := config.Load()
	b.WriteString("## Configuration\n\n")
	if cfgErr != nil {
		c := checkResult{"Config", statusFail, "failed to resolve: " + cfgErr.Error()}
		out.Configuration = append(out.Configuration, c)
		out.Verdict = statusFail
		b.WriteString(formatCheck(c) + "\n")
		return finish(true)
	}
	if cfg == nil {
		c := checkResult{"Config", statusFail, "not initialized"}
		out.Configuration = append(out.Configuration, c)
		out.Verdict = statusFail
		out.Hint = "Run `init_moodle_context` to initialize."
		b.WriteString(formatCheck(c) + "\n")
		b.WriteString("\n✖ Run `init_moodle_context` to initialize.\n")
		return finish(false)
	}
	configFilePath, cfgErr := config.FilePath()
	if cfgErr != nil {
		c := checkResult{"Config file", statusFail, "failed to resolve: " + cfgErr.Error()}
		out.Configuration = append(out.Configuration, c)
		out.Verdict = statusFail
		b.WriteString(formatCheck(c) + "\n")
		return finish(true)
	}
	out.Configuration = []checkResult{
		{"Config file", statusOK, configFilePath},
		{"Moodle path", statusOK, cfg.MoodlePath},
		{"Moodle version", statusOK, cfg.MoodleVersion},
	}
	fmt.Fprintf(&b, "%s\n", formatCheck(out.Configuration[0]))
	fmt.Fprintf(&b, "%s\n", formatCheck(out.Configuration[1]))
	fmt.Fprintf(&b, "%s\n\n", formatCheck(out.Configuration[2]))

	// 3. Moodle installation.
	b.WriteString("## Moodle Installation\n\n")
	var installChecks []checkResult
	if !dirExists(cfg.MoodlePath) {
		installChecks = append(installChecks, checkResult{"Installation directory", statusFail, "does not exist"})
	} else if !extractors.IsMoodleRoot(cfg.MoodlePath) {
		installChecks = append(installChecks, checkResult{"Installation directory", statusFail, "does not look like a valid Moodle root"})
	} else {
		installChecks = append(installChecks, checkResult{"Installation directory", statusOK, ""})
	}
	out.MoodleInstallation = installChecks
	for _, c := range installChecks {
		fmt.Fprintln(&b, formatCheck(c))
	}
	b.WriteString("\n")

	// 4. Global index files.
	globalChecks := checkFreshness(func(f string) string { return generators.GlobalOutputPath(cfg.MoodlePath, f) }, expectedGlobalFiles)
	out.GlobalIndexFiles = globalChecks
	b.WriteString("## Global Index Files\n\n")
	for _, c := range globalChecks {
		fmt.Fprintln(&b, formatCheck(c))
	}
	b.WriteString("\n")

	// 5. Development plugins.
	devDirs := generators.FindDevPlugins(cfg.MoodlePath)
	sort.Strings(devDirs)
	devPlugins := loadDevPlugins(cfg.MoodlePath, devDirs)
	var pluginChecks []checkResult
	b.WriteString("## Development Plugins\n\n")
	if len(devDirs) == 0 {
		b.WriteString("  _(no .indevelopment plugins)_\n\n")
	} else {
		for _, p := range devPlugins {
			component := p.component()
			fmt.Fprintf(&b, "  Plugin: %s\n", component)
			checks := checkFreshness(func(f string) string { return generators.PluginOutputPath(p.dir, f) }, generators.PluginContextFiles)
			pluginChecks = append(pluginChecks, checks...)
			out.DevelopmentPlugins = append(out.DevelopmentPlugins, DoctorPluginChecks{Component: component, Checks: checks})
			for _, c := range checks {
				fmt.Fprintln(&b, "  "+formatCheck(c))
			}
		}
		b.WriteString("\n")
	}

	// 6. Legacy files pending migration (read-only).
	b.WriteString("## Legacy Files Pending Migration\n\n")
	legacyCheck := legacyFilesCheck(len(generators.DetectLegacyGlobalFiles(cfg.MoodlePath)), func() int {
		n := 0
		for _, dir := range devDirs {
			n += len(generators.DetectLegacyPluginFiles(dir))
		}
		return n
	}())
	out.LegacyFiles = []checkResult{legacyCheck}
	fmt.Fprintf(&b, "%s\n\n", formatCheck(legacyCheck))

	noDevPlugins := "  _(nothing to check — no dev plugins)_"
	out.CrossPluginConsistency = checkCrossPluginConsistency(devPlugins)
	writeCheckSection(&b, "Cross-Plugin Consistency", out.CrossPluginConsistency,
		"  _(nothing to check — fewer than 2 dev plugins)_")

	// Deprecated core API usage: the global @deprecated function set against every dev plugin's
	// PHP source.
	out.DeprecatedApiUsage = checkDeprecatedApiUsage(cfg.MoodlePath, devPlugins)
	writeCheckSection(&b, "Deprecated Core API Usage", out.DeprecatedApiUsage, noDevPlugins)

	// Capability usage: each dev plugin's db/access.php declarations against its own
	// has_capability()/require_capability() calls.
	out.CapabilityUsage = checkCapabilityUsage(devPlugins)
	writeCheckSection(&b, "Capability Usage", out.CapabilityUsage, noDevPlugins)

	// Lang string usage: each dev plugin's own get_string() calls against lang/en/{component}.php.
	out.LangStringUsage = checkLangStringUsage(devPlugins)
	writeCheckSection(&b, "Lang String Usage", out.LangStringUsage, noDevPlugins)

	// Upgrade consistency: each dev plugin's db/upgrade.php against db/install.xml and version.php.
	out.UpgradeConsistency = checkUpgradeConsistency(devPlugins)
	writeCheckSection(&b, "Upgrade Consistency", out.UpgradeConsistency, noDevPlugins)

	// Privacy API: each dev plugin's classes/privacy/provider.php against its tables.
	out.PrivacyApi = checkPrivacyApi(devPlugins)
	writeCheckSection(&b, "Privacy API", out.PrivacyApi, noDevPlugins)

	// 7. Cache.
	stats := cache.Global.Stats()
	out.Cache = &DoctorCache{Hits: stats.Hits, Misses: stats.Misses, Skips: stats.Skips}
	b.WriteString("## Cache\n\n")
	fmt.Fprintf(&b, "  Hits: %d, Misses: %d, Skips: %d\n", stats.Hits, stats.Misses, stats.Skips)
	cachePath := generators.GlobalOutputPath(cfg.MoodlePath, ".cache.json")
	if info, err := os.Stat(cachePath); err == nil {
		out.Cache.File = cachePath
		out.Cache.FileBytes = info.Size()
		fmt.Fprintf(&b, "  Cache file: %s (%d bytes)\n\n", cachePath, info.Size())
	} else {
		fmt.Fprintf(&b, "  Cache file: not yet created\n\n")
	}

	groups := [][]checkResult{deps, installChecks, globalChecks, pluginChecks, {legacyCheck}, out.CrossPluginConsistency, out.DeprecatedApiUsage, out.CapabilityUsage, out.LangStringUsage, out.UpgradeConsistency, out.PrivacyApi}
	out.Verdict = verdictStatus(groups...)
	b.WriteString(computeVerdict(groups...))

	return finish(false)
}

// normalize replaces nil section slices with empty ones so every section is always present in
// the JSON response as an array, never as null.
func (o *DoctorOutput) normalize() {
	for _, s := range []*[]checkResult{
		&o.SystemDependencies, &o.Configuration, &o.MoodleInstallation, &o.GlobalIndexFiles,
		&o.LegacyFiles, &o.CrossPluginConsistency, &o.DeprecatedApiUsage, &o.CapabilityUsage,
		&o.LangStringUsage, &o.UpgradeConsistency, &o.PrivacyApi,
	} {
		if *s == nil {
			*s = []checkResult{}
		}
	}
	if o.DevelopmentPlugins == nil {
		o.DevelopmentPlugins = []DoctorPluginChecks{}
	}
}

// checkSystemDependencies reports whether the optional external tools php, ctags, git and phpcs
// are on PATH, looking them up concurrently; a ctags that is not Universal Ctags is reported as
// unusable, since the tags file is then skipped, and so is a phpcs that does not run or lacks the
// moodle coding standard, since check_coding_style then cannot work. A missing or unusable tool is
// a warning, never a failure.
func checkSystemDependencies() []checkResult {
	tools := []string{"php", "ctags", "git", "phpcs"}
	results := make([]checkResult, len(tools))
	var wg sync.WaitGroup
	wg.Add(len(tools))
	for i, t := range tools {
		go func(i int, t string) {
			defer wg.Done()
			if t == "ctags" {
				switch found, universal := generators.CtagsStatus(); {
				case universal:
					results[i] = checkResult{t, statusOK, "found"}
				case found:
					results[i] = checkResult{t, statusWarn, "found, but not Universal Ctags — the tags file is skipped (optional)"}
				default:
					results[i] = checkResult{t, statusWarn, "not found (optional)"}
				}
				return
			}
			if t == "phpcs" {
				results[i] = phpcsDependency()
				return
			}
			if _, err := exec.LookPath(t); err == nil {
				results[i] = checkResult{t, statusOK, "found"}
			} else {
				results[i] = checkResult{t, statusWarn, "not found (optional)"}
			}
		}(i, t)
	}
	wg.Wait()
	return results
}

// phpcsDependency returns the system-dependency check of phpcs (see phpcs.Check): ok when it
// runs with the moodle standard installed, a warning otherwise.
func phpcsDependency() checkResult {
	switch s := phpcs.Check(context.Background()); s.State {
	case phpcs.Ready:
		return checkResult{"phpcs", statusOK, "found, with the moodle coding standard"}
	case phpcs.NoStandard:
		return checkResult{"phpcs", statusWarn, "found, but the moodle coding standard is not installed — check_coding_style is unavailable (optional)"}
	case phpcs.Unusable:
		return checkResult{"phpcs", statusWarn, "found, but it does not work (" + s.Detail + ") — check_coding_style is unavailable (optional)"}
	}
	return checkResult{"phpcs", statusWarn, "not found (optional, used by check_coding_style)"}
}

// checkFreshness checks that each of `files` exists at the path returned by `pathFor` and reports
// its age: missing files fail, files older than staleThresholdDays warn, others pass. The results
// are in the same order as `files`.
func checkFreshness(pathFor func(string) string, files []string) []checkResult {
	results := make([]checkResult, len(files))
	for i, f := range files {
		info, err := os.Stat(pathFor(f))
		if err != nil {
			results[i] = checkResult{f, statusFail, "missing"}
			continue
		}
		age := time.Since(info.ModTime())
		days := int(age.Hours() / 24)
		if days > staleThresholdDays {
			results[i] = checkResult{f, statusWarn, fmt.Sprintf("%dd ago (stale)", days)}
			continue
		}
		if days == 0 {
			results[i] = checkResult{f, statusOK, "today"}
		} else {
			results[i] = checkResult{f, statusOK, fmt.Sprintf("%dd ago", days)}
		}
	}
	return results
}

// checkCrossPluginConsistency flags capabilities declared identically by more than one dev
// plugin, since Moodle requires capability names to be globally unique.
func checkCrossPluginConsistency(devPlugins []devPlugin) []checkResult {
	if len(devPlugins) < 2 {
		return nil
	}
	owners := map[string][]string{}
	for _, p := range devPlugins {
		if p.caps == nil {
			continue
		}
		component := p.component()
		for _, c := range p.caps.Capabilities {
			owners[c.Name] = append(owners[c.Name], component)
		}
	}

	var names []string
	for name, comps := range owners {
		if len(comps) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	if len(names) == 0 {
		return []checkResult{{"Capabilities", statusOK, "no duplicate capability names across dev plugins"}}
	}
	var results []checkResult
	for _, name := range names {
		sort.Strings(owners[name])
		results = append(results, checkResult{
			"Capabilities", statusWarn,
			fmt.Sprintf("%q declared by multiple plugins: %s", name, strings.Join(owners[name], ", ")),
		})
	}
	return results
}

// legacyFilesCheck returns the doctor check for `global` legacy files at the Moodle root and
// `plugin` legacy files in development plugins, naming the tools whose next run moves each kind
// into .build82/: init_moodle_context or update_indexes for the global files, generate_plugin_context
// or plugin_batch (or the watcher) for a plugin's files.
func legacyFilesCheck(global, plugin int) checkResult {
	var pending []string
	if global > 0 {
		pending = append(pending, fmt.Sprintf("%d global file(s) — moved into .build82/ on the next init_moodle_context or update_indexes run", global))
	}
	if plugin > 0 {
		pending = append(pending, fmt.Sprintf("%d plugin file(s) — moved into the plugin's .build82/ on its next generate_plugin_context or plugin_batch run (or watcher regeneration)", plugin))
	}
	if len(pending) == 0 {
		return checkResult{"Legacy files", statusOK, "No legacy files pending migration."}
	}
	return checkResult{"Legacy files", statusWarn, strings.Join(pending, "; ")}
}

// writeCheckSection writes the "## `title`" section of the doctor report to `b`: one line per entry
// of `checks`, or the `emptyNote` line when there are none (every check function returns nil when
// it has nothing to check).
func writeCheckSection(b *strings.Builder, title string, checks []checkResult, emptyNote string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(checks) == 0 {
		b.WriteString(emptyNote + "\n\n")
		return
	}
	for _, c := range checks {
		fmt.Fprintln(b, formatCheck(c))
	}
	b.WriteString("\n")
}

// devPlugin is one .indevelopment plugin as doctor's checks read it: detected once, with the
// extractions that more than one check needs.
type devPlugin struct {
	dir      string
	relDir   string // dir relative to the Moodle root, with forward slashes
	info     extractors.PluginInfo
	detected bool // extractors.DetectPlugin succeeded; info is the zero value otherwise
	caps     *extractors.CapabilitiesExtraction
	schema   *extractors.DbSchema
	phpFiles []extractors.PhpFile // the plugin's *.php files, listed once for every source scan
}

// component returns the plugin's component, or its directory relative to the Moodle root when it
// could not be detected, so no absolute host path reaches the report.
func (p devPlugin) component() string {
	if p.detected {
		return p.info.Component
	}
	return p.relDir
}

// detectPlugin is extractors.DetectPlugin, as a variable so tests can count detections.
var detectPlugin = extractors.DetectPlugin

// loadDevPlugins detects each plugin directory of `dirs` (inside the Moodle root `moodlePath`),
// extracts its capabilities and database schema and lists its *.php files, keeping the order of
// `dirs`.
func loadDevPlugins(moodlePath string, dirs []string) []devPlugin {
	plugins := make([]devPlugin, len(dirs))
	for i, dir := range dirs {
		info, err := detectPlugin(dir)
		plugins[i] = devPlugin{
			dir:      dir,
			relDir:   relativeToMoodle(moodlePath, dir),
			info:     info,
			detected: err == nil,
			caps:     extractors.ExtractPluginCapabilities(dir),
			schema:   extractors.ExtractPluginSchema(dir),
			phpFiles: extractors.ListPhpFiles(dir),
		}
	}
	return plugins
}

// checkCapabilityUsage compares, for each plugin in `devPlugins`, the has_capability() and
// require_capability() calls that name one of the plugin's own capabilities (by prefix) with the
// capabilities declared in db/access.php. A call naming an undeclared capability is reported as a
// warning, since it likely contains a typo and would silently behave as "no permission". It
// returns nil when `devPlugins` is empty and a single passing result when nothing is wrong.
func checkCapabilityUsage(devPlugins []devPlugin) []checkResult {
	if len(devPlugins) == 0 {
		return nil
	}

	var results []checkResult
	for _, p := range devPlugins {
		if !p.detected {
			continue
		}
		info := p.info

		declared := map[string]struct{}{}
		if p.caps != nil {
			for _, c := range p.caps.Capabilities {
				declared[c.Name] = struct{}{}
			}
		}

		ownPrefix := extractors.CapabilityPrefix(info.Component)
		if ownPrefix == "" {
			continue
		}
		calls := extractors.FindOwnCapabilityChecksIn(p.phpFiles, ownPrefix)
		sort.Slice(calls, func(i, j int) bool {
			if calls[i].File != calls[j].File {
				return calls[i].File < calls[j].File
			}
			return calls[i].Line < calls[j].Line
		})
		for _, c := range calls {
			if _, ok := declared[c.Capability]; ok {
				continue
			}
			results = append(results, checkResult{
				"Capability Usage", statusWarn,
				fmt.Sprintf("%s checks %q at %s:%d, which is not declared in db/access.php (typo?)", info.Component, c.Capability, c.File, c.Line),
			})
		}
	}
	if len(results) == 0 {
		return []checkResult{{"Capability Usage", statusOK, "no undeclared own-capability checks found in dev plugins"}}
	}
	return results
}

// checkLangStringUsage compares, for each plugin in `devPlugins`, the get_string() calls
// naming one of the plugin's own strings (by component) with the strings declared in
// lang/en/{component}.php. A string used but not declared is reported as a warning, since it
// would render as a missing-string placeholder. It returns nil when `devPlugins` is empty and a
// single passing result when nothing is wrong.
func checkLangStringUsage(devPlugins []devPlugin) []checkResult {
	if len(devPlugins) == 0 {
		return nil
	}

	var results []checkResult
	for _, p := range devPlugins {
		if !p.detected {
			continue
		}
		dir, info := p.dir, p.info

		declared := extractors.ExtractLangStrings(dir, info.Component)
		calls := extractors.FindOwnGetStringCallsIn(p.phpFiles, info.Component)
		sort.Slice(calls, func(i, j int) bool {
			if calls[i].File != calls[j].File {
				return calls[i].File < calls[j].File
			}
			return calls[i].Line < calls[j].Line
		})
		for _, c := range calls {
			if _, ok := declared[c.Identifier]; ok {
				continue
			}
			results = append(results, checkResult{
				"Lang String Usage", statusWarn,
				fmt.Sprintf("%s calls get_string(%q, ...) at %s:%d, which is not declared in lang/en/%s.php",
					info.Component, c.Identifier, c.File, c.Line, moodletype.LangFileName(info.Component)),
			})
		}
	}
	if len(results) == 0 {
		return []checkResult{{"Lang String Usage", statusOK, "no undeclared own-lang-string calls found in dev plugins"}}
	}
	return results
}

// checkDeprecatedApiUsage reports, as warnings, the calls that the PHP source of each plugin
// in `devPlugins` makes to core functions marked @deprecated under `moodlePath`. It returns
// nil when `devPlugins` is empty and a single passing result when no deprecated call is found.
func checkDeprecatedApiUsage(moodlePath string, devPlugins []devPlugin) []checkResult {
	if len(devPlugins) == 0 {
		return nil
	}

	deprecated := deprecatedFunctionNames(moodlePath)
	if len(deprecated) == 0 {
		return []checkResult{{"Deprecated API", statusOK, "no deprecated core functions found in this Moodle version"}}
	}

	var results []checkResult
	for _, p := range devPlugins {
		component := p.component()
		calls := extractors.FindDeprecatedApiUsageIn(p.phpFiles, deprecated)
		sort.Slice(calls, func(i, j int) bool {
			if calls[i].File != calls[j].File {
				return calls[i].File < calls[j].File
			}
			return calls[i].Line < calls[j].Line
		})
		for _, c := range calls {
			results = append(results, checkResult{
				"Deprecated API", statusWarn,
				fmt.Sprintf("%s calls deprecated `%s()` at %s:%d", component, c.Function, c.File, c.Line),
			})
		}
	}
	if len(results) == 0 {
		return []checkResult{{"Deprecated API", statusOK, "no deprecated core API calls found in dev plugins"}}
	}
	return results
}

// apiIndexFunctionLinePattern matches the leading "- `name()`" of a function line of the API index
// and captures the function name.
var apiIndexFunctionLinePattern = regexp.MustCompile("^- `([a-zA-Z_][a-zA-Z0-9_]*)\\(\\)`")

// deprecatedFunctionNames returns the set of core function names marked @deprecated for the Moodle
// installation at `moodlePath`. It reads the generated MOODLE_API_INDEX.md when it exists, which is
// much cheaper than parsing every file under lib/, and otherwise falls back to
// extractors.ExtractMoodleApi. A stale index is reported separately by the "Global Index Files"
// freshness check.
func deprecatedFunctionNames(moodlePath string) map[string]struct{} {
	if content, err := fsutil.ReadRegular(generators.GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md"), 0); err == nil {
		return parseDeprecatedNamesFromIndex(string(content))
	}
	deprecated := map[string]struct{}{}
	for _, f := range extractors.ExtractMoodleApi(moodlePath).Functions {
		if f.Visibility == extractors.VisDeprecated {
			deprecated[f.Name] = struct{}{}
		}
	}
	return deprecated
}

// parseDeprecatedNamesFromIndex returns the names of the functions whose line in the API index
// `content` carries an @deprecated marker.
func parseDeprecatedNamesFromIndex(content string) map[string]struct{} {
	deprecated := map[string]struct{}{}
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "@deprecated") {
			continue
		}
		if m := apiIndexFunctionLinePattern.FindStringSubmatch(line); m != nil {
			deprecated[m[1]] = struct{}{}
		}
	}
	return deprecated
}

// verdictStatus reduces every check to the worst status found: fail beats warn beats ok.
func verdictStatus(groups ...[]checkResult) checkStatus {
	hasWarn := false
	for _, g := range groups {
		for _, c := range g {
			switch c.Status {
			case statusFail:
				return statusFail
			case statusWarn:
				hasWarn = true
			}
		}
	}
	if hasWarn {
		return statusWarn
	}
	return statusOK
}

// computeVerdict renders the overall verdict line for `groups` based on verdictStatus.
func computeVerdict(groups ...[]checkResult) string {
	switch verdictStatus(groups...) {
	case statusFail:
		return "❌ Issues found\n"
	case statusWarn:
		return "⚠️ Warnings found\n"
	default:
		return "✅ All checks passed.\n"
	}
}

// checkUpgradeConsistency checks, for each plugin in `devPlugins`, that db/upgrade.php,
// db/install.xml and version.php agree (see upgradeConsistencyIssues). It returns a single ok
// result when no issue is found, and nil when `devPlugins` is empty.
func checkUpgradeConsistency(devPlugins []devPlugin) []checkResult {
	if len(devPlugins) == 0 {
		return nil
	}
	var results []checkResult
	for _, p := range devPlugins {
		if !p.detected {
			continue
		}
		results = append(results, upgradeConsistencyIssues(p.dir, p.info, p.schema)...)
	}
	if len(results) == 0 {
		return []checkResult{{"Upgrade Consistency", statusOK, "db/upgrade.php, db/install.xml and version.php agree in every dev plugin"}}
	}
	return results
}

// UpgradeFinding is one upgrade-consistency problem of a plugin: its severity ("fail" or "warn")
// and its description, which names the component and the db/upgrade.php line.
type UpgradeFinding struct {
	Status string
	Detail string
}

// UpgradeConsistencyFindings returns the upgrade-consistency problems of the plugin at
// `pluginPath` (see upgradeConsistencyIssues), or nil when there is none or the plugin cannot be
// detected.
func UpgradeConsistencyFindings(pluginPath string) []UpgradeFinding {
	info, err := extractors.DetectPlugin(pluginPath)
	if err != nil {
		return nil
	}
	var findings []UpgradeFinding
	for _, r := range upgradeConsistencyIssues(pluginPath, info, extractors.ExtractPluginSchema(pluginPath)) {
		findings = append(findings, UpgradeFinding{Status: string(r.Status), Detail: r.Detail})
	}
	return findings
}

// DeprecatedFunctionNames returns the core functions marked @deprecated in the Moodle installation
// at `moodlePath` (see deprecatedFunctionNames).
func DeprecatedFunctionNames(moodlePath string) map[string]struct{} {
	return deprecatedFunctionNames(moodlePath)
}

// savepointFor returns the savepoint function an upgrade step of a plugin of type `pluginType`
// named `name` must call, and the string arguments it takes after the version:
// upgrade_mod_savepoint(true, V, name) for activity modules, upgrade_block_savepoint(true, V, name)
// for blocks, and upgrade_plugin_savepoint(true, V, type, name) for every other plugin type.
func savepointFor(pluginType, name string) (function string, args []string) {
	switch pluginType {
	case "mod":
		return "upgrade_mod_savepoint", []string{name}
	case "block":
		return "upgrade_block_savepoint", []string{name}
	}
	return "upgrade_plugin_savepoint", []string{pluginType, name}
}

// sameVersion reports whether the version numbers `a` and `b` are equal numerically (so
// "2024010100" equals "2024010100.00"); ok is false when either is not a number.
func sameVersion(a, b string) (equal, ok bool) {
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return false, false
	}
	return x == y, true
}

// upgradeConsistencyIssues returns the problems found in the plugin at `dir` (detected as `info`)
// between its db/upgrade.php steps, db/install.xml and version.php, each as a "Upgrade
// Consistency" check naming the component and the db/upgrade.php line:
//   - fail: a step without a savepoint, or whose last savepoint is for another version;
//   - fail: a savepoint function or arguments that do not match the plugin type (savepointFor);
//   - fail: steps whose versions do not strictly increase;
//   - fail: a step newer than $plugin->version in version.php;
//   - the final schema the steps leave behind, compared with db/install.xml (reportFinalSchema).
//
// A plugin without db/upgrade.php has no issue. Whether db/install.xml gained a table without a
// matching step cannot be told from the files alone and is not checked.
func upgradeConsistencyIssues(dir string, info extractors.PluginInfo, schema *extractors.DbSchema) []checkResult {
	ops := extractors.ParseUpgradeOperations(dir)
	if ops == nil || len(ops.Blocks) == 0 {
		return nil
	}
	tables := map[string]map[string]bool{}
	if schema != nil {
		for _, t := range schema.Tables {
			fields := map[string]bool{}
			for _, f := range t.Fields {
				fields[f.Name] = true
			}
			tables[t.Name] = fields
		}
	}
	var issues []checkResult
	report := func(status checkStatus, line int, format string, args ...any) {
		detail := fmt.Sprintf("%s: %s:%d: %s", info.Component, ops.File, line, fmt.Sprintf(format, args...))
		issues = append(issues, checkResult{"Upgrade Consistency", status, detail})
	}

	// The plugin type and name come from the component (type_name), which identifies the plugin
	// the way the savepoint functions do.
	pluginType, pluginName, _ := strings.Cut(info.Component, "_")
	wantFn, wantArgs := savepointFor(pluginType, pluginName)
	var previous float64
	for i, b := range ops.Blocks {
		v, err := strconv.ParseFloat(b.Version, 64)
		if err == nil && i > 0 && v <= previous {
			report(statusFail, b.Line, "upgrade step %s does not come after the previous step's version (steps must increase)", b.Version)
		}
		if err == nil {
			previous = v
		}

		if len(b.Savepoints) == 0 {
			report(statusFail, b.Line, "upgrade step %s has no savepoint call", b.Version)
		} else {
			sp := b.Savepoints[len(b.Savepoints)-1]
			if equal, ok := sameVersion(sp.Version, b.Version); ok && !equal {
				report(statusFail, sp.Line, "upgrade step %s ends with a savepoint for version %s", b.Version, sp.Version)
			}
			if sp.Function != wantFn || !slices.Equal(sp.Args, wantArgs) {
				report(statusFail, sp.Line, "upgrade step %s calls %s(true, %s, %s); a %s plugin must call %s(true, %s, %s)",
					b.Version, sp.Function, sp.Version, quoteArgs(sp.Args), pluginType, wantFn, b.Version, quoteArgs(wantArgs))
			}
		}
	}

	reportFinalSchema(ops.Blocks, tables, report)

	if pv, err := strconv.ParseFloat(info.Version, 64); err == nil {
		for _, b := range ops.Blocks {
			if v, err := strconv.ParseFloat(b.Version, 64); err == nil && v > pv {
				report(statusFail, b.Line, "upgrade step %s is newer than $plugin->version %s in version.php, so it never runs", b.Version, info.Version)
			}
		}
	}
	return issues
}

// quoteArgs renders `args` as single-quoted PHP string arguments separated by ", ".
func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + a + "'"
	}
	return strings.Join(quoted, ", ")
}

// schemaState is the last operation the upgrade steps performed on one table or field: whether it
// leaves the object present, the index of the step that did it and the operation's line.
type schemaState struct {
	present    bool
	step, line int
}

// reportFinalSchema replays the schema operations of `blocks` in order and compares the state they
// leave behind with `tables` (db/install.xml: table name → field names), reporting through
// `report`:
//   - fail: a table that ends up created (or renamed to) but that db/install.xml lacks, or a field
//     that ends up added (or renamed to) but that its db/install.xml table lacks — a fresh install
//     would miss it;
//   - warn: a table that ends up dropped (or renamed from) but that db/install.xml still defines,
//     or such a field that its db/install.xml table still has.
//
// Operations are replayed in source order and only the last one on each name counts, so a field
// added and later renamed or dropped is judged by its final state. A name that a later step still writes as a string literal (for
// example through a helper function) is left out, since its final state cannot be told.
func reportFinalSchema(blocks []extractors.UpgradeBlock, tables map[string]map[string]bool, report func(checkStatus, int, string, ...any)) {
	tableState := map[string]schemaState{}
	fieldState := map[extractors.TableField]schemaState{}
	for i, b := range blocks {
		for _, op := range b.Ops {
			field := extractors.TableField{Table: op.Table, Field: op.Field}
			switch op.Op {
			case "create_table":
				tableState[op.Table] = schemaState{true, i, op.Line}
			case "drop_table":
				tableState[op.Table] = schemaState{false, i, op.Line}
			case "rename_table":
				tableState[op.Table] = schemaState{false, i, op.Line}
				tableState[op.NewName] = schemaState{true, i, op.Line}
			case "add_field":
				fieldState[field] = schemaState{true, i, op.Line}
			case "drop_field":
				fieldState[field] = schemaState{false, i, op.Line}
			case "rename_field":
				fieldState[field] = schemaState{false, i, op.Line}
				fieldState[extractors.TableField{Table: op.Table, Field: op.NewName}] = schemaState{true, i, op.Line}
			}
		}
	}
	// mentionedLater reports whether a step after `step` writes `name` as a string literal.
	mentionedLater := func(name string, step int) bool {
		for _, b := range blocks[step+1:] {
			if b.Literals[name] {
				return true
			}
		}
		return false
	}

	names := make([]string, 0, len(tableState))
	for t := range tableState {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		st := tableState[t]
		b := blocks[st.step]
		_, defined := tables[t]
		switch {
		case mentionedLater(t, st.step):
		case st.present && !defined:
			report(statusFail, st.line, "upgrade step %s leaves table %s in place, but db/install.xml does not define it (a fresh install would lack it)", b.Version, t)
		case !st.present && defined:
			report(statusWarn, st.line, "upgrade step %s removes table %s, but db/install.xml still defines it", b.Version, t)
		}
	}

	fields := make([]extractors.TableField, 0, len(fieldState))
	for f := range fieldState {
		fields = append(fields, f)
	}
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Table != fields[j].Table {
			return fields[i].Table < fields[j].Table
		}
		return fields[i].Field < fields[j].Field
	})
	for _, f := range fields {
		st := fieldState[f]
		b := blocks[st.step]
		defined, ok := tables[f.Table]
		switch {
		case !ok || mentionedLater(f.Field, st.step):
		case st.present && !defined[f.Field]:
			report(statusFail, st.line, "upgrade step %s leaves field %s.%s in place, but db/install.xml does not define it (a fresh install would lack it)", b.Version, f.Table, f.Field)
		case !st.present && defined[f.Field]:
			report(statusWarn, st.line, "upgrade step %s removes field %s.%s, but db/install.xml still defines it", b.Version, f.Table, f.Field)
		}
	}
}

// userDataFields are the field names that mark a table as storing data about a user.
var userDataFields = map[string]bool{"userid": true, "usermodified": true, "relateduserid": true}

// checkPrivacyApi checks the Privacy API provider of each plugin in `devPlugins`:
//   - warn: classes/privacy/provider.php is missing;
//   - warn: it implements neither \core_privacy\local\metadata\null_provider nor
//     \core_privacy\local\metadata\provider;
//   - warn: it is a null_provider, but a db/install.xml table has a user field (userid,
//     usermodified, relateduserid) or a foreign key to the user table, so the plugin stores personal
//     data it does not declare.
//
// It returns a single ok result when no issue is found, and nil when `devPlugins` is empty.
func checkPrivacyApi(devPlugins []devPlugin) []checkResult {
	if len(devPlugins) == 0 {
		return nil
	}
	var results []checkResult
	warn := func(format string, args ...any) {
		results = append(results, checkResult{"Privacy API", statusWarn, fmt.Sprintf(format, args...)})
	}
	for _, dp := range devPlugins {
		if !dp.detected {
			continue
		}
		info := dp.info
		p := extractors.DetectPrivacyProvider(dp.dir)
		switch {
		case !p.Exists:
			warn("%s: no %s — every plugin must declare a Privacy API provider (\\core_privacy\\local\\metadata\\null_provider when it stores no personal data)", info.Component, p.File)
			continue
		case !p.IsNull && !p.IsMetadata:
			warn("%s: %s implements neither \\core_privacy\\local\\metadata\\null_provider nor \\core_privacy\\local\\metadata\\provider", info.Component, p.File)
			continue
		case p.IsMetadata:
			continue
		}
		if table, field, ok := firstUserDataField(dp.schema); ok {
			warn("%s: %s declares a null_provider, but db/install.xml table %s stores user data (%s); declare it with \\core_privacy\\local\\metadata\\provider", info.Component, p.File, table, field)
		}
	}
	if len(results) == 0 {
		return []checkResult{{"Privacy API", statusOK, "every dev plugin declares a Privacy API provider consistent with its tables"}}
	}
	return results
}

// firstUserDataField returns the first db/install.xml table of the plugin at `dir`, in file order,
// that stores user data, with what gives it away: a field from userDataFields, or a foreign key
// referencing the user table. ok is false when there is none.
func firstUserDataField(schema *extractors.DbSchema) (table, evidence string, ok bool) {
	if schema == nil {
		return "", "", false
	}
	for _, t := range schema.Tables {
		for _, f := range t.Fields {
			if userDataFields[f.Name] {
				return t.Name, "field " + f.Name, true
			}
		}
		for _, k := range t.Keys {
			if k.Ref == "user" {
				return t.Name, "foreign key " + k.Name + " to user", true
			}
		}
	}
	return "", "", false
}
