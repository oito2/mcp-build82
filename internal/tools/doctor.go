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

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
)

type checkStatus string

const (
	statusOK   checkStatus = "ok"
	statusWarn checkStatus = "warn"
	statusFail checkStatus = "fail"
)

type checkResult struct {
	Label  string      `json:"label"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

func formatCheck(c checkResult) string {
	icon := map[checkStatus]string{statusOK: "✔", statusWarn: "⚠", statusFail: "✖"}[c.Status]
	detail := ""
	if c.Detail != "" {
		detail = " — " + c.Detail
	}
	return fmt.Sprintf("  %s %s%s", icon, c.Label, detail)
}

const staleThresholdDays = 7

var expectedGlobalFiles = generators.GlobalContextFilenames

type DoctorInput struct {
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// DoctorPluginChecks groups the index-freshness checks of one development plugin.
type DoctorPluginChecks struct {
	Component string        `json:"component"`
	Checks    []checkResult `json:"checks"`
}

// DoctorCache reports the in-process cache counters and the on-disk cache file, if any.
type DoctorCache struct {
	Hits      int    `json:"hits"`
	Misses    int    `json:"misses"`
	Skips     int    `json:"skips"`
	File      string `json:"file,omitempty"`
	FileBytes int64  `json:"file_bytes,omitempty"`
}

// DoctorOutput is the structured form of the doctor report: one field per report section, plus
// an overall verdict ("ok", "warn" or "fail"). Sections that could not run because the
// configuration is missing or unresolvable are left empty.
type DoctorOutput struct {
	SystemDependencies     []checkResult        `json:"system_dependencies"`
	Configuration          []checkResult        `json:"configuration"`
	MoodleInstallation     []checkResult        `json:"moodle_installation"`
	GlobalIndexFiles       []checkResult        `json:"global_index_files"`
	DevelopmentPlugins     []DoctorPluginChecks `json:"development_plugins"`
	LegacyFiles            []checkResult        `json:"legacy_files"`
	CrossPluginConsistency []checkResult        `json:"cross_plugin_consistency"`
	DeprecatedApiUsage     []checkResult        `json:"deprecated_api_usage"`
	CapabilityUsage        []checkResult        `json:"capability_usage"`
	LangStringUsage        []checkResult        `json:"lang_string_usage"`
	Cache                  *DoctorCache         `json:"cache,omitempty"`
	Verdict                checkStatus          `json:"verdict"`
	Hint                   string               `json:"hint,omitempty"`
}

func RegisterDoctorTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "doctor",
		Description: "Runs a full environment diagnostic: system dependencies, configuration, Moodle " +
			"installation, generated index freshness, dev plugins, legacy files pending migration, " +
			"cross-plugin consistency, deprecated core API usage, own-capability check consistency, " +
			"own-lang-string usage consistency, and cache stats.",
	}, withRecover(handleDoctor))
}

func handleDoctor(ctx context.Context, req *mcp.CallToolRequest, in DoctorInput) (*mcp.CallToolResult, struct{}, error) {
	var b strings.Builder
	out := DoctorOutput{}
	finish := func(isError bool) (*mcp.CallToolResult, struct{}, error) {
		if in.Format == FormatJSON {
			out.normalize()
			return jsonResult(isError, out), struct{}{}, nil
		}
		return textResult(isError, b.String()), struct{}{}, nil
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
	var pluginChecks []checkResult
	b.WriteString("## Development Plugins\n\n")
	if len(devDirs) == 0 {
		b.WriteString("  _(no .indevelopment plugins)_\n\n")
	} else {
		for _, dir := range devDirs {
			component := dir
			if info, err := extractors.DetectPlugin(dir); err == nil {
				component = info.Component
			}
			fmt.Fprintf(&b, "  Plugin: %s\n", component)
			checks := checkFreshness(func(f string) string { return generators.PluginOutputPath(dir, f) }, generators.PluginContextFiles)
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
	legacyCount := len(generators.DetectLegacyGlobalFiles(cfg.MoodlePath))
	for _, dir := range devDirs {
		legacyCount += len(generators.DetectLegacyPluginFiles(dir))
	}
	var legacyCheck checkResult
	if legacyCount == 0 {
		legacyCheck = checkResult{"Legacy files", statusOK, "No legacy files pending migration."}
	} else {
		legacyCheck = checkResult{"Legacy files", statusWarn, fmt.Sprintf("%d file(s) found — will be moved into .build82/ automatically on the next init_moodle_context or update_indexes run", legacyCount)}
	}
	out.LegacyFiles = []checkResult{legacyCheck}
	fmt.Fprintf(&b, "%s\n\n", formatCheck(legacyCheck))

	crossPluginChecks := checkCrossPluginConsistency(devDirs)
	out.CrossPluginConsistency = crossPluginChecks
	b.WriteString("## Cross-Plugin Consistency\n\n")
	if len(crossPluginChecks) == 0 {
		b.WriteString("  _(nothing to check — fewer than 2 dev plugins)_\n\n")
	} else {
		for _, c := range crossPluginChecks {
			fmt.Fprintln(&b, formatCheck(c))
		}
		b.WriteString("\n")
	}

	// Deprecated core API usage: cross-references the global @deprecated function set against
	// every dev plugin's own PHP source.
	deprecatedChecks := checkDeprecatedApiUsage(cfg.MoodlePath, devDirs)
	out.DeprecatedApiUsage = deprecatedChecks
	b.WriteString("## Deprecated Core API Usage\n\n")
	if len(devDirs) == 0 {
		b.WriteString("  _(nothing to check — no dev plugins)_\n\n")
	} else {
		for _, c := range deprecatedChecks {
			fmt.Fprintln(&b, formatCheck(c))
		}
		b.WriteString("\n")
	}

	// Capability usage: cross-references each dev plugin's own db/access.php declarations against
	// has_capability()/require_capability() calls naming one of its own capabilities.
	capabilityUsageChecks := checkCapabilityUsage(devDirs)
	out.CapabilityUsage = capabilityUsageChecks
	b.WriteString("## Capability Usage\n\n")
	if len(devDirs) == 0 {
		b.WriteString("  _(nothing to check — no dev plugins)_\n\n")
	} else {
		for _, c := range capabilityUsageChecks {
			fmt.Fprintln(&b, formatCheck(c))
		}
		b.WriteString("\n")
	}

	// Lang string usage: cross-references each dev plugin's own get_string() calls naming one of
	// its own lang strings against what lang/en/{component}.php actually declares.
	langStringUsageChecks := checkLangStringUsage(devDirs)
	out.LangStringUsage = langStringUsageChecks
	b.WriteString("## Lang String Usage\n\n")
	if len(devDirs) == 0 {
		b.WriteString("  _(nothing to check — no dev plugins)_\n\n")
	} else {
		for _, c := range langStringUsageChecks {
			fmt.Fprintln(&b, formatCheck(c))
		}
		b.WriteString("\n")
	}

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

	groups := [][]checkResult{deps, installChecks, globalChecks, pluginChecks, {legacyCheck}, crossPluginChecks, deprecatedChecks, capabilityUsageChecks, langStringUsageChecks}
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
		&o.LangStringUsage,
	} {
		if *s == nil {
			*s = []checkResult{}
		}
	}
	if o.DevelopmentPlugins == nil {
		o.DevelopmentPlugins = []DoctorPluginChecks{}
	}
}

func checkSystemDependencies() []checkResult {
	tools := []string{"php", "ctags", "git"}
	results := make([]checkResult, len(tools))
	var wg sync.WaitGroup
	wg.Add(len(tools))
	for i, t := range tools {
		go func(i int, t string) {
			defer wg.Done()
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
func checkCrossPluginConsistency(devDirs []string) []checkResult {
	if len(devDirs) < 2 {
		return nil
	}
	owners := map[string][]string{}
	for _, dir := range devDirs {
		caps := extractors.ExtractPluginCapabilities(dir)
		if caps == nil {
			continue
		}
		component := dir
		if info, err := extractors.DetectPlugin(dir); err == nil {
			component = info.Component
		}
		for _, c := range caps.Capabilities {
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

// checkCapabilityUsage cross-references each dev plugin's own has_capability()/require_capability()
// calls (naming one of its own capabilities, by prefix) against what it actually declares in
// db/access.php — catches a typo in the checked capability name, which would otherwise only
// surface at runtime as an "access denied" (a check for an undeclared capability behaves like "no
// permission", not like an error).
func checkCapabilityUsage(devDirs []string) []checkResult {
	if len(devDirs) == 0 {
		return nil
	}

	var results []checkResult
	for _, dir := range devDirs {
		info, err := extractors.DetectPlugin(dir)
		if err != nil {
			continue
		}

		declared := map[string]struct{}{}
		if caps := extractors.ExtractPluginCapabilities(dir); caps != nil {
			for _, c := range caps.Capabilities {
				declared[c.Name] = struct{}{}
			}
		}

		ownPrefix := extractors.CapabilityPrefix(info.Type, info.Name, info.Component)
		calls := extractors.FindOwnCapabilityChecks(dir, ownPrefix)
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

// checkLangStringUsage cross-references each dev plugin's own get_string() calls (naming one of
// its own lang strings, by component) against what's actually declared in
// lang/en/{component}.php — catches a lang string used in code but never defined, which fails at
// runtime with a "string not found" / [[missingstring]] placeholder.
func checkLangStringUsage(devDirs []string) []checkResult {
	if len(devDirs) == 0 {
		return nil
	}

	var results []checkResult
	for _, dir := range devDirs {
		info, err := extractors.DetectPlugin(dir)
		if err != nil {
			continue
		}

		declared := extractors.ExtractLangStrings(dir, info.Component)
		calls := extractors.FindOwnGetStringCalls(dir, info.Component)
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
					info.Component, c.Identifier, c.File, c.Line, info.Component),
			})
		}
	}
	if len(results) == 0 {
		return []checkResult{{"Lang String Usage", statusOK, "no undeclared own-lang-string calls found in dev plugins"}}
	}
	return results
}

// checkDeprecatedApiUsage cross-references core functions marked @deprecated in the global API
// index against calls made by each dev plugin's own PHP source, so plugin authors catch
// compatibility drift before it surfaces as a runtime deprecation notice in Moodle itself.
func checkDeprecatedApiUsage(moodlePath string, devDirs []string) []checkResult {
	if len(devDirs) == 0 {
		return nil
	}

	deprecated := deprecatedFunctionNames(moodlePath)
	if len(deprecated) == 0 {
		return []checkResult{{"Deprecated API", statusOK, "no deprecated core functions found in this Moodle version"}}
	}

	var results []checkResult
	for _, dir := range devDirs {
		component := dir
		if info, err := extractors.DetectPlugin(dir); err == nil {
			component = info.Component
		}
		calls := extractors.FindDeprecatedApiUsage(dir, deprecated)
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

// apiIndexFunctionLinePattern matches one apiFunctionLine-rendered line's leading "- `name()`" —
// the exact format apiFunctionLine produces, which search_api's substring matching also relies on.
var apiIndexFunctionLinePattern = regexp.MustCompile("^- `([a-zA-Z_][a-zA-Z0-9_]*)\\(\\)`")

// deprecatedFunctionNames prefers the already-generated, already-cached MOODLE_API_INDEX.md
// (produced by init_moodle_context/update_indexes) over a full live re-parse of every file in
// {moodlePath}/lib — doctor is meant to be a fast diagnostic, and re-running
// extractors.ExtractMoodleApi on every doctor call would duplicate that same expensive work on
// every single invocation, dozens of files each time (per this project's own benchmark, a single
// ~10k-line core file alone costs ~377ms with the tree-sitter backend). Falls back to the live
// extraction when the index doesn't exist yet, matching every
// other cache-aware read in this codebase's "never fatal, just slower" degradation contract — a
// stale index (the Moodle core was upgraded since the last update_indexes run) is caught
// separately by doctor's own "Global Index Files" freshness check above, not silently masked here.
func deprecatedFunctionNames(moodlePath string) map[string]struct{} {
	if content, err := os.ReadFile(generators.GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md")); err == nil {
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
