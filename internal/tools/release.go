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
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// excludedNames is the fixed exclusion set: the whole .build82/ directory, the legacy bare
// PLUGIN_*.md/.indevelopment filenames (for plugins not yet migrated — release_plugin never
// triggers migration itself), and AI-assistant context files unrelated to build82's own output.
var excludedNames = buildExcludedNames()

func buildExcludedNames() map[string]struct{} {
	m := map[string]struct{}{
		generators.ContextDir: {},
		"CLAUDE.md":           {}, "GEMINI.md": {}, "AGENTS.md": {},
		".claudeignore": {}, ".geminiignore": {}, ".aiexclude": {},
		"node_modules": {},
		// .buildignore itself is a build82-specific config file, not part of the plugin — shipping
		// it inside the release ZIP by default would be an oversight, the same reasoning that
		// already excludes .claudeignore/.geminiignore/.aiexclude above.
		".buildignore": {},
		// .git is excluded unconditionally, in every mode — not just flagged as an error under
		// strict:true (validateForRelease below also reports it as an explicit issue). A plugin under active development commonly has a .git directory
		// inside it; packaging it into the default (non-strict) release ZIP would ship the entire
		// commit history — including anything ever committed and later removed — to every
		// installer of that ZIP.
		".git": {},
	}
	for _, f := range generators.PluginContextFiles {
		m[f] = struct{}{}
	}
	m[".indevelopment"] = struct{}{}
	return m
}

type ReleasePluginInput struct {
	Component string `json:"component" jsonschema:"Component (e.g. 'local_myplugin'), path relative to the Moodle root (e.g. 'local/myplugin'), or absolute path"`
	// output_dir, not outputDir — every other tool input field in this codebase is snake_case
	// (moodle_path, plugin_path, mark_as_dev, ...); this was the one camelCase outlier, safe to
	// rename since no v1.0.0 has been published yet.
	OutputDir string `json:"output_dir,omitempty" jsonschema:"Directory to write the ZIP into (defaults to the current working directory; must already exist)"`
	Strict    bool   `json:"strict,omitempty" jsonschema:"Validate moodle.org plugin directory submission requirements before packaging; refuses to build the ZIP if any fail (default: false, package unconditionally)"`
}

func RegisterReleaseTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "release_plugin",
		Description: "Packages a plugin directory into a distributable ZIP, excluding build82's own " +
			"generated files and other non-shippable artifacts. With strict:true, also validates the " +
			"plugin against moodle.org plugin directory submission requirements before packaging.",
	}, withRecover(handleReleasePlugin))
}

func handleReleasePlugin(ctx context.Context, req *mcp.CallToolRequest, in ReleasePluginInput) (*mcp.CallToolResult, struct{}, error) {
	if strings.TrimSpace(in.Component) == "" {
		return textResult(true, "❌ component is required: a component (e.g. 'local_myplugin'), a path relative to the Moodle root (e.g. 'local/myplugin'), or an absolute path."), struct{}{}, nil
	}
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}

	outputDir := in.OutputDir
	if outputDir == "" {
		outputDir, _ = os.Getwd()
	} else if !dirExists(outputDir) {
		return textResult(true, fmt.Sprintf("❌ Output directory does not exist: %s", outputDir)), struct{}{}, nil
	}

	// The same central helper generate_plugin_context/explain_plugin use: it accepts all three
	// identifier forms and rejects anything that resolves outside the Moodle root (e.g.
	// "local/../../etc").
	rp, errResult := resolveAndValidatePlugin(in.Component, cfg.MoodlePath)
	if errResult != nil {
		return errResult, struct{}{}, nil
	}
	pluginPath, info := rp.Path, rp.Info
	if info.Version == "" {
		return textResult(true, fmt.Sprintf("❌ Could not read version from %s/version.php", relativeToMoodle(cfg.MoodlePath, pluginPath))), struct{}{}, nil
	}

	if in.Strict {
		if issues := validateForRelease(pluginPath, requestedComponent(in.Component, info), info); len(issues) > 0 {
			var b strings.Builder
			b.WriteString("❌ Not ready for moodle.org submission — fix the following before packaging:\n\n")
			for _, issue := range issues {
				fmt.Fprintf(&b, "- %s\n", issue)
			}
			return textResult(true, b.String()), struct{}{}, nil
		}
	}

	// Use info.Component (extracted from version.php by resolveAndValidatePlugin, above) rather
	// than the raw in.Component from the MCP client, which may be a path: building the ZIP
	// filename from client-controlled input is an unnecessary risk. info.Component is plain
	// PHP-identifier-shaped data read from the plugin's own source.
	zipName := fmt.Sprintf("%s_%s.zip", info.Component, info.Version)
	destination := filepath.Join(outputDir, zipName)
	folderName := filepath.Base(pluginPath)

	excluded := mergeBuildIgnore(pluginPath, excludedNames)
	foundExcluded, err := createZip(pluginPath, destination, folderName, excluded)
	if err != nil {
		return textResult(true, "❌ Failed to create ZIP: "+err.Error()), struct{}{}, nil
	}

	var b strings.Builder
	// Paths are reported relative to the Moodle root, never absolute. A ZIP written outside the
	// Moodle root (the usual case: output_dir or the working directory) is reported by its file
	// name alone, which is its path relative to output_dir — the caller already knows that
	// directory, and echoing it back would leak the host's directory layout.
	output, ok := displayPathWithinMoodle(cfg.MoodlePath, destination)
	if !ok {
		output = zipName
	}
	fmt.Fprintf(&b, "✅ Released %s (version %s).\n\n", info.Component, info.Version)
	fmt.Fprintf(&b, "ZIP folder: %s/\nOutput: %s\nSource: %s\n\n", folderName, output, relativeToMoodle(cfg.MoodlePath, pluginPath))
	if len(foundExcluded) > 0 {
		sort.Strings(foundExcluded)
		b.WriteString("Excluded from the archive:\n")
		for _, name := range foundExcluded {
			fmt.Fprintf(&b, "- %s\n", name)
		}
	}
	return textResult(false, b.String()), struct{}{}, nil
}

// requestedComponent returns the component the caller asked to release, for strict mode's
// "version.php declares the expected component" check. A component-shaped identifier is taken
// as-is; a path identifier names no component itself, so the one implied by the plugin's
// location on disk ({type}_{directory name}) is used instead.
func requestedComponent(identifier string, info extractors.PluginInfo) string {
	if !filepath.IsAbs(identifier) && !strings.ContainsAny(identifier, `/\`) {
		return identifier
	}
	return info.Type + "_" + info.Name
}

// validMoodleMaturities are the four maturity constants a plugin's $plugin->maturity may use:
// MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC and MATURITY_STABLE.
var validMoodleMaturities = map[string]struct{}{
	"MATURITY_ALPHA": {}, "MATURITY_BETA": {}, "MATURITY_RC": {}, "MATURITY_STABLE": {},
}

// validateForRelease checks a minimal, objectively-verifiable set of release requirements: the
// requested component matches what version.php itself declares, the required version.php fields are present and well-formed, the component's English
// language file exists, a Privacy API provider is declared, third-party libraries are declared if
// present, and no .git directory got left inside the plugin folder. It deliberately stops short of
// subjective checks (README quality, code style) that would need heuristics instead of a
// straightforward yes/no per requirement.
func validateForRelease(pluginPath, requestedComponent string, info extractors.PluginInfo) []string {
	var issues []string

	if info.Component != requestedComponent {
		issues = append(issues, fmt.Sprintf(
			"version.php declares $plugin->component = %q but the plugin was resolved as %q — moodle.org requires these to match exactly",
			info.Component, requestedComponent))
	}
	if info.Requires == "" {
		issues = append(issues, "version.php is missing $plugin->requires")
	}
	if info.Maturity == "" {
		issues = append(issues, "version.php is missing $plugin->maturity")
	} else if _, ok := validMoodleMaturities[info.Maturity]; !ok {
		issues = append(issues, fmt.Sprintf("version.php declares an unrecognized $plugin->maturity value %q (expected one of MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC, MATURITY_STABLE)", info.Maturity))
	}

	langFile := filepath.Join(pluginPath, "lang", "en", info.Component+".php")
	if !fileExists(langFile) {
		issues = append(issues, fmt.Sprintf("missing language file lang/en/%s.php", info.Component))
	}

	// Every plugin must declare a Privacy API provider — even one that stores no personal data
	// must implement \core_privacy\local\metadata\null_provider explicitly rather than omit the
	// class. This is a plain file-existence check.
	if !fileExists(filepath.Join(pluginPath, "classes", "privacy", "provider.php")) {
		issues = append(issues, "missing classes/privacy/provider.php — every plugin must declare a Privacy API "+
			"provider (implement \\core_privacy\\local\\metadata\\null_provider if the plugin stores no personal data)")
	}

	// thirdpartylibs.xml is required whenever a plugin bundles third-party code under thirdparty/,
	// declaring name/version/license/location for each bundled library.
	if dirExists(filepath.Join(pluginPath, "thirdparty")) && !fileExists(filepath.Join(pluginPath, "thirdpartylibs.xml")) {
		issues = append(issues, "plugin has a thirdparty/ directory but no thirdpartylibs.xml declaring the bundled libraries")
	}

	if dirExists(filepath.Join(pluginPath, ".git")) {
		issues = append(issues, "plugin directory contains a .git directory — remove it before submitting to the moodle.org plugin directory")
	}

	return issues
}

// mergeBuildIgnore adds any patterns from the plugin's optional .buildignore file to the fixed
// exclusion set, additively — a missing file is not an error, just nothing extra to merge.
// Patterns are matched as exact basenames, .gitignore-style directory/file names, one per line,
// '#'-comments and blank lines skipped.
func mergeBuildIgnore(pluginPath string, base map[string]struct{}) map[string]struct{} {
	merged := make(map[string]struct{}, len(base))
	for k := range base {
		merged[k] = struct{}{}
	}
	content, err := os.ReadFile(filepath.Join(pluginPath, ".buildignore"))
	if err != nil {
		return merged
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		merged[line] = struct{}{}
	}
	return merged
}

// newZipDestination opens the on-disk file createZip writes into (always a temporary file next to
// the final ZIP, never the final path itself). It's a package-level variable (rather than a direct
// os.Create call) purely so tests can substitute a destination whose Write()/Close() fails, to
// verify createZip surfaces that failure and leaves no partial archive behind. Production
// behavior: os.Create.
var newZipDestination = func(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

// createZip recursively collects pluginPath's files, skipping any entry whose basename is in
// excluded (at every depth), and archives them under folderName/ at outputPath. Returns the
// basenames that were actually found and excluded (for reporting).
//
// The archive is written to a temporary file in outputPath's directory and renamed onto
// outputPath only after it has been completely written and closed, so a failure at any point
// (walk, read, write, or the final central-directory flush) never leaves a truncated ZIP at
// outputPath — nor replaces a previous good one. The temporary file is removed on failure.
func createZip(pluginPath, outputPath, folderName string, excluded map[string]struct{}) ([]string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	// os.CreateTemp creates the file 0600; a release archive is meant to be shared, so give it the
	// usual 0644 a plain os.Create would typically have produced.
	chmodErr := tmp.Chmod(0o644)
	if err := errors.Join(chmodErr, tmp.Close()); err != nil {
		os.Remove(tmpPath)
		return nil, err
	}

	found, err := writeZip(pluginPath, tmpPath, folderName, excluded, []string{tmpPath, outputPath})
	if err == nil {
		err = os.Rename(tmpPath, outputPath)
	}
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	return found, nil
}

// writeZip does the actual archiving for createZip into destPath. Entries whose path is in skip
// (the archive being written, and its final destination) are never archived into themselves when
// the output directory lies inside the plugin.
//
// Both f.Close() and zw.Close() errors are captured via the named err return (rather than being
// discarded by a bare `defer f.Close()`/`defer zw.Close()`) — zip.Writer.Close() is what actually
// flushes the ZIP's central directory, so if it fails (e.g. a full disk or another I/O error), the
// archive is truncated/corrupt even though every individual file write up to that point
// succeeded. Silently ignoring that error would let release_plugin report success for a broken ZIP.
// Only the first Close() error is kept — a later Close() failing after an earlier real error (from
// the walk itself, or from the first Close()) must not mask that earlier, more specific error.
func writeZip(pluginPath, destPath, folderName string, excluded map[string]struct{}, skip []string) (found []string, err error) {
	f, err := newZipDestination(destPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if closeErr := zw.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	skipSet := make(map[string]struct{}, len(skip))
	for _, p := range skip {
		if abs, absErr := filepath.Abs(p); absErr == nil {
			skipSet[abs] = struct{}{}
		}
	}
	foundSet := map[string]struct{}{}

	walkErr := filepath.WalkDir(pluginPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == pluginPath {
			return nil
		}
		if _, isExcluded := excluded[d.Name()]; isExcluded {
			foundSet[d.Name()] = struct{}{}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if abs, absErr := filepath.Abs(path); absErr == nil {
			if _, isSelf := skipSet[abs]; isSelf {
				return nil
			}
		}
		rel, relErr := filepath.Rel(pluginPath, path)
		if relErr != nil {
			return relErr
		}
		w, createErr := zw.Create(filepath.ToSlash(filepath.Join(folderName, rel)))
		if createErr != nil {
			return createErr
		}
		src, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer src.Close()
		_, copyErr := io.Copy(w, src)
		return copyErr
	})
	if walkErr != nil {
		return nil, walkErr
	}

	found = make([]string, 0, len(foundSet))
	for name := range foundSet {
		found = append(found, name)
	}
	return found, nil
}
