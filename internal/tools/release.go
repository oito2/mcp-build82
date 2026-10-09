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
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// excludedNames is the fixed set of base names never packaged into a release ZIP: the .build82/
// directory, the bare PLUGIN_*.md and .indevelopment files of plugins that still use the legacy
// layout, other AI-assistant context files, node_modules, .buildignore and .git.
var excludedNames = buildExcludedNames()

// buildExcludedNames builds the fixed exclusion set held in excludedNames.
func buildExcludedNames() map[string]struct{} {
	m := map[string]struct{}{
		generators.ContextDir: {},
		"CLAUDE.md":           {}, "GEMINI.md": {}, "AGENTS.md": {},
		".claudeignore": {}, ".geminiignore": {}, ".aiexclude": {},
		"node_modules": {},
		// .buildignore is build82 configuration, not part of the plugin, like the AI ignore files.
		".buildignore": {},
		// .git is excluded in every mode (strict mode additionally reports it as an issue),
		// because packaging it would ship the full commit history, including removed content.
		".git": {},
	}
	for _, f := range generators.PluginContextFiles {
		m[f] = struct{}{}
	}
	m[".indevelopment"] = struct{}{}
	return m
}

// ReleasePluginInput is the input of the release_plugin tool.
type ReleasePluginInput struct {
	Component string `json:"component" jsonschema:"Component (e.g. 'local_myplugin'), path relative to the Moodle root (e.g. 'local/myplugin'), or absolute path"`
	OutputDir string `json:"output_dir,omitempty" jsonschema:"Directory to write the ZIP into (defaults to the current working directory; must already exist)"`
	Strict    bool   `json:"strict,omitempty" jsonschema:"Validate moodle.org plugin directory submission requirements before packaging; refuses to build the ZIP if any fail (default: false, package unconditionally)"`
}

// RegisterReleaseTool registers the release_plugin tool on `server`.
func RegisterReleaseTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "release_plugin",
		Annotations: toolAnnotations("Package Plugin Release", false, true, true, false),
		Description: "Packages a plugin directory into a distributable ZIP, excluding build82's own " +
			"generated files and other non-shippable artifacts. With strict:true, also validates the " +
			"plugin against moodle.org plugin directory submission requirements before packaging.",
	}, withRecover(handleReleasePlugin))
}

// handleReleasePlugin packages the plugin named by `in.Component` into
// {component}_{version}.zip inside `in.OutputDir` (default: the working directory, which must
// exist), excluding build82's own files. With `in.Strict` it first validates the release
// requirements and refuses to build when any fail. It returns an error result when the component
// is empty, the configuration or plugin is invalid, the output directory is missing, validation
// fails, or the ZIP cannot be written; the error return is always nil.
func handleReleasePlugin(ctx context.Context, req *mcp.CallToolRequest, in ReleasePluginInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Component) == "" {
		return textResult(true, "❌ component is required: a component (e.g. 'local_myplugin'), a path relative to the Moodle root (e.g. 'local/myplugin'), or an absolute path."), nil, nil
	}
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), nil, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), nil, nil
	}

	outputDir := in.OutputDir
	if outputDir == "" {
		outputDir, _ = os.Getwd()
	} else if !dirExists(outputDir) {
		return textResult(true, fmt.Sprintf("❌ Output directory does not exist: %s", outputDir)), nil, nil
	}

	// Accepts a component, relative path or absolute path, and rejects anything resolving outside
	// the Moodle root (e.g. "local/../../etc").
	rp, errResult := resolveAndValidatePlugin(in.Component, cfg.MoodlePath)
	if errResult != nil {
		return errResult, nil, nil
	}
	pluginPath, info := rp.Path, rp.Info
	if info.Version == "" {
		return textResult(true, fmt.Sprintf("❌ Could not read version from %s/version.php", relativeToMoodle(cfg.MoodlePath, pluginPath))), nil, nil
	}

	if in.Strict {
		if issues := validateForRelease(pluginPath, requestedComponent(in.Component, info), info); len(issues) > 0 {
			var b strings.Builder
			b.WriteString("❌ Not ready for moodle.org submission — fix the following before packaging:\n\n")
			for _, issue := range issues {
				fmt.Fprintf(&b, "- %s\n", issue)
			}
			return textResult(true, b.String()), nil, nil
		}
	}

	// The file name uses info.Component (read from the plugin's version.php) rather than the raw
	// client-supplied identifier, which may be a path.
	zipName := fmt.Sprintf("%s_%s.zip", info.Component, info.Version)
	destination := filepath.Join(outputDir, zipName)
	folderName := filepath.Base(pluginPath)

	excluded := mergeBuildIgnore(pluginPath, excludedNames)
	foundExcluded, skippedLinks, err := createZip(pluginPath, destination, folderName, excluded)
	if err != nil {
		return textResult(true, "❌ Failed to create ZIP: "+err.Error()), nil, nil
	}

	var b strings.Builder
	// Paths are reported relative to the Moodle root, never absolute. A ZIP written outside the
	// Moodle root (the usual case) is reported by its file name alone, so the host's directory
	// layout is not exposed.
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
	if len(skippedLinks) > 0 {
		sort.Strings(skippedLinks)
		b.WriteString("\n⚠️ Symbolic links skipped (never added to the archive):\n")
		for _, rel := range skippedLinks {
			fmt.Fprintf(&b, "- %s\n", rel)
		}
	}
	return textResult(false, b.String()), nil, nil
}

// requestedComponent returns the component the caller asked to release, used by strict mode to
// check that version.php declares the expected component. A component-shaped `identifier` is
// returned as is; a path identifier names no component itself, so the one implied by the
// plugin's location ({type}_{directory name}, from `info`) is used instead.
func requestedComponent(identifier string, info extractors.PluginInfo) string {
	if !filepath.IsAbs(identifier) && !strings.ContainsAny(identifier, `/\`) {
		return identifier
	}
	return info.Type + "_" + info.Name
}

// validMoodleMaturities is the set of accepted $plugin->maturity values: MATURITY_ALPHA,
// MATURITY_BETA, MATURITY_RC and MATURITY_STABLE.
var validMoodleMaturities = map[string]struct{}{
	"MATURITY_ALPHA": {}, "MATURITY_BETA": {}, "MATURITY_RC": {}, "MATURITY_STABLE": {},
}

// validateForRelease checks the plugin at `pluginPath` (described by `info`) against a minimal set
// of objectively verifiable release requirements: `requestedComponent` matches the component
// declared in version.php, the required version.php fields are present and well-formed, the
// English language file exists, a Privacy API provider exists, bundled third-party libraries are
// declared, and no .git directory is present. Subjective checks such as README quality or code
// style are out of scope. It returns one message per failed requirement, or nil when all pass.
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

	// Every plugin must declare a Privacy API provider; one that stores no personal data
	// implements null_provider. This is a plain file-existence check.
	if !fileExists(filepath.Join(pluginPath, "classes", "privacy", "provider.php")) {
		issues = append(issues, "missing classes/privacy/provider.php — every plugin must declare a Privacy API "+
			"provider (implement \\core_privacy\\local\\metadata\\null_provider if the plugin stores no personal data)")
	}

	// thirdpartylibs.xml must declare the libraries whenever third-party code is bundled under
	// thirdparty/.
	if dirExists(filepath.Join(pluginPath, "thirdparty")) && !fileExists(filepath.Join(pluginPath, "thirdpartylibs.xml")) {
		issues = append(issues, "plugin has a thirdparty/ directory but no thirdpartylibs.xml declaring the bundled libraries")
	}

	if dirExists(filepath.Join(pluginPath, ".git")) {
		issues = append(issues, "plugin directory contains a .git directory — remove it before submitting to the moodle.org plugin directory")
	}

	return issues
}

// mergeBuildIgnore returns a copy of the exclusion set `base` extended with the entries of the
// plugin's optional .buildignore file at `pluginPath`. Each line is an exact base name of a file
// or directory; blank lines and lines starting with '#' are skipped. A missing or unreadable file
// adds nothing. `base` is never modified.
func mergeBuildIgnore(pluginPath string, base map[string]struct{}) map[string]struct{} {
	merged := make(map[string]struct{}, len(base))
	for k := range base {
		merged[k] = struct{}{}
	}
	content, err := fsutil.ReadRegular(filepath.Join(pluginPath, ".buildignore"), 0)
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

// newZipDestination opens the file at `path` for writing the archive; createZip passes a
// temporary file next to the final ZIP. It defaults to os.Create and is a variable so tests can
// substitute a destination whose Write or Close fails.
var newZipDestination = func(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

// createZip archives the files under `pluginPath`, skipping every entry whose base name is in
// `excluded` at any depth, into the ZIP at `outputPath`, with all entries placed under
// `folderName`/. Symbolic links (files and directories) are never archived. It returns the base
// names that were found and excluded, the plugin-relative paths of the skipped symbolic links, and
// an error when the archive cannot be created or written.
//
// The archive is written to a temporary file in outputPath's directory and renamed onto
// `outputPath` only after it has been completely written and closed, so a failure never leaves a
// truncated ZIP there nor replaces a previous good one. The temporary file is removed on failure.
func createZip(pluginPath, outputPath, folderName string, excluded map[string]struct{}) (found, links []string, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return nil, nil, err
	}
	tmpPath := tmp.Name()
	// os.CreateTemp creates the file with mode 0600; a release archive is meant to be shared, so
	// it gets the usual 0644.
	chmodErr := tmp.Chmod(0o644)
	if err := errors.Join(chmodErr, tmp.Close()); err != nil {
		os.Remove(tmpPath)
		return nil, nil, err
	}

	found, links, err = writeZip(pluginPath, tmpPath, folderName, excluded, []string{tmpPath, outputPath})
	if err == nil {
		err = os.Rename(tmpPath, outputPath)
	}
	if err != nil {
		os.Remove(tmpPath)
		return nil, nil, err
	}
	return found, links, nil
}

// writeZip writes the archive described in createZip to `destPath` and returns the base names that
// were found and excluded plus the plugin-relative (slash-separated) paths of the symbolic links
// that were skipped. Files whose path is in `skip` (the archive being written and its final
// destination) are not archived, so the archive never contains itself when the output directory
// lies inside the plugin.
//
// Errors from closing the file and the zip writer are returned through `err`, because closing the
// zip writer is what flushes the central directory; ignoring that failure would report success for
// a corrupt archive. Only the first error is kept so a later close failure never masks an earlier,
// more specific one.
func writeZip(pluginPath, destPath, folderName string, excluded map[string]struct{}, skip []string) (found, links []string, err error) {
	f, err := newZipDestination(destPath)
	if err != nil {
		return nil, nil, err
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
		if d.Type()&fs.ModeSymlink != 0 {
			rel, relErr := filepath.Rel(pluginPath, path)
			if relErr != nil {
				return relErr
			}
			links = append(links, filepath.ToSlash(rel))
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
		src, openErr := fsutil.OpenRegular(path)
		if openErr != nil {
			return openErr
		}
		defer src.Close()
		_, copyErr := io.Copy(w, src)
		return copyErr
	})
	if walkErr != nil {
		return nil, nil, walkErr
	}

	found = make([]string, 0, len(foundSet))
	for name := range foundSet {
		found = append(found, name)
	}
	return found, links, nil
}
