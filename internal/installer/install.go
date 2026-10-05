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

package installer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/fsutil"
)

// Test seams: the operating system the path table is built for, PATH lookups used for
// detection, and the runner for the tools' own CLIs (`claude`, `codex`). Tests replace these so
// they never depend on the host OS or run a real third-party CLI.
var (
	goos       = runtime.GOOS
	lookPath   = exec.LookPath
	runCommand = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
)

type configShape string

const (
	shapeMcpServers configShape = "mcpServers" // {"mcpServers": {"build82": {command, args, env}}}
	shapeCursor     configShape = "cursor"     // {"mcpServers": {"build82": {type: "stdio", command, args, env}}}
	shapeOpenCode   configShape = "opencode"   // {"mcp": {"build82": {type: "local", command: [...], environment, enabled}}}
	shapeZed        configShape = "zed"        // {"context_servers": {"build82": {command, args, env}}}
	shapeCLI        configShape = "cli"        // not a JSON file at all — the tool's own `mcp add` command
)

// topKey returns the top-level JSON object key a build82 entry lives under for this shape. This
// is the single source of truth for the configShape -> JSON key mapping, shared by the install
// and uninstall sides.
func (s configShape) topKey() string {
	switch s {
	case shapeMcpServers, shapeCursor:
		return "mcpServers"
	case shapeOpenCode:
		return "mcp"
	case shapeZed:
		return "context_servers"
	}
	return ""
}

// cliSpec describes a tool that manages its own MCP registrations through a CLI, so build82
// delegates to that CLI instead of editing the tool's config file directly.
type cliSpec struct {
	Bin     string
	AddArgs func(binaryPath, moodlePath string) []string
	// RemoveArgs returns the command that removes build82 from one scope. Tools with a single
	// configuration receive an empty scope and ignore it.
	RemoveArgs func(scope string) []string
	// GetArgs must exit with status 0 only when build82 is registered.
	GetArgs []string
	// Scope, when set, marks a tool that keeps registrations in several scopes and extracts from
	// the GetArgs output the scope of the registration the tool reports. It returns "" when the
	// output names no known scope.
	Scope func(getOutput []byte) string
}

// Claude Code scopes, as accepted by `claude mcp remove --scope`.
const (
	scopeLocal   = "local"
	scopeProject = "project"
	scopeUser    = "user"
)

// claudeScope reads the scope from `claude mcp get build82` output. The command reports only the
// registration that takes precedence (local over project over user) as a line such as
// "  Scope: User config (available in all your projects)", and ends with a hint such as
// "To remove this server, run: claude mcp remove build82 -s user", which is used as a fallback.
func claudeScope(out []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < len("scope:") || !strings.EqualFold(line[:len("scope:")], "scope:") {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(line[len("scope:"):]))
		for _, s := range []string{scopeLocal, scopeProject, scopeUser} {
			if strings.HasPrefix(value, s) {
				return s
			}
		}
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		for i, f := range fields {
			var value string
			switch {
			case (f == "-s" || f == "--scope") && i+1 < len(fields):
				value = fields[i+1]
			case strings.HasPrefix(f, "--scope="):
				value = strings.TrimPrefix(f, "--scope=")
			default:
				continue
			}
			value = strings.ToLower(strings.Trim(value, "`'\".,"))
			if value == scopeLocal || value == scopeProject || value == scopeUser {
				return value
			}
		}
	}
	return ""
}

// target is one entry in the install table.
type target struct {
	ID, Label string
	// A target is detected when DetectCmd is on PATH or any directory from DetectDirs exists.
	// Either may be empty.
	DetectCmd  string
	DetectDirs func() []string
	// InstallPaths lists the config files an install merges the build82 entry into. It may
	// depend on which of the tool's directories currently exist.
	InstallPaths func() []string
	// RemovePaths lists every config file an uninstall inspects and cleans, including
	// legacy locations that are cleaned on uninstall. nil means InstallPaths.
	RemovePaths func() []string
	Shape       configShape
	CLI         *cliSpec // for Shape == shapeCLI
	// Unsupported, when non-empty, explains why this target cannot be used on the current OS.
	Unsupported string
}

func (t target) removePaths() []string {
	if t.RemovePaths != nil {
		return t.RemovePaths()
	}
	if t.InstallPaths != nil {
		return t.InstallPaths()
	}
	return nil
}

func fixedPaths(paths ...string) func() []string {
	return func() []string { return paths }
}

// targets builds the install table. A home directory that cannot be resolved (unset
// $HOME/%USERPROFILE%) is an error: defaulting to "" would make every path below relative to the
// process's cwd, so install/uninstall could touch an unrelated file with no visible error.
func targets() ([]target, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	vscodeCline, err := clineDir()
	if err != nil {
		return nil, err
	}
	appData := appDataDir(home)

	claudeDesktop := target{ID: "claude-desktop", Label: "Claude Desktop", Shape: shapeMcpServers}
	switch goos {
	case "darwin":
		claudeDesktop.DetectDirs = fixedPaths(filepath.Join(home, "Library", "Application Support", "Claude"))
	case "windows":
		claudeDesktop.DetectDirs = fixedPaths(filepath.Join(appData, "Claude"))
	case "linux":
		claudeDesktop.DetectDirs = fixedPaths(filepath.Join(xdgConfigHome(home), "Claude"))
	default:
		claudeDesktop.Unsupported = "Claude Desktop is only available for macOS, Windows and Linux"
	}
	if claudeDesktop.DetectDirs != nil {
		file := filepath.Join(claudeDesktop.DetectDirs()[0], "claude_desktop_config.json")
		claudeDesktop.InstallPaths = fixedPaths(file)
	}

	geminiConfig := filepath.Join(home, ".gemini", "config")

	opencodeDir := filepath.Join(home, ".config", "opencode")
	opencodeJSON := filepath.Join(opencodeDir, "opencode.json")
	opencodeJSONC := filepath.Join(opencodeDir, "opencode.jsonc")
	opencodeLegacy := filepath.Join(opencodeDir, "config.json")

	zedDir := filepath.Join(home, ".config", "zed")
	if goos == "windows" {
		zedDir = filepath.Join(appData, "Zed")
	}

	// clineCLIDir is the directory whose presence marks a Cline CLI install; clineCLIData holds
	// its settings. CLINE_DATA_DIR replaces the default ~/.cline/data as a whole.
	clineCLIDir := filepath.Join(home, ".cline")
	clineCLIData := filepath.Join(clineCLIDir, "data")
	if v := os.Getenv("CLINE_DATA_DIR"); v != "" {
		clineCLIDir, clineCLIData = v, v
	}

	return []target{
		{ID: "claude", Label: "Claude Code", DetectCmd: "claude", Shape: shapeCLI, CLI: &cliSpec{
			Bin: "claude",
			AddArgs: func(binaryPath, moodlePath string) []string {
				return []string{"mcp", "add", "--scope", "user", "build82",
					"-e", "BUILD82_MOODLE_PATH=" + moodlePath, "--", binaryPath}
			},
			RemoveArgs: func(scope string) []string {
				return []string{"mcp", "remove", "--scope", scope, "build82"}
			},
			GetArgs: []string{"mcp", "get", "build82"},
			Scope:   claudeScope,
		}},
		claudeDesktop,
		{ID: "antigravity", Label: "Antigravity (IDE / CLI)", DetectCmd: "agy",
			DetectDirs:   fixedPaths(geminiConfig, filepath.Join(home, ".gemini", "antigravity")),
			InstallPaths: fixedPaths(filepath.Join(geminiConfig, "mcp_config.json")),
			RemovePaths: fixedPaths(filepath.Join(geminiConfig, "mcp_config.json"),
				filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")),
			Shape: shapeMcpServers},
		{ID: "codex", Label: "OpenAI Codex CLI", DetectCmd: "codex", Shape: shapeCLI, CLI: &cliSpec{
			Bin: "codex",
			AddArgs: func(binaryPath, moodlePath string) []string {
				return []string{"mcp", "add", "build82",
					"--env", "BUILD82_MOODLE_PATH=" + moodlePath, "--", binaryPath}
			},
			RemoveArgs: func(string) []string { return []string{"mcp", "remove", "build82"} },
			GetArgs:    []string{"mcp", "get", "build82"},
		}},
		{ID: "opencode", Label: "OpenCode", DetectCmd: "opencode",
			InstallPaths: func() []string {
				// OpenCode reads either opencode.json or opencode.jsonc; reuse an existing .jsonc
				// rather than creating a second global config file next to it.
				if fileExists(opencodeJSONC) && !fileExists(opencodeJSON) {
					return []string{opencodeJSONC}
				}
				return []string{opencodeJSON}
			},
			RemovePaths: fixedPaths(opencodeJSON, opencodeJSONC, opencodeLegacy),
			Shape:       shapeOpenCode},
		{ID: "cursor", Label: "Cursor",
			DetectDirs:   fixedPaths(filepath.Join(home, ".cursor")),
			InstallPaths: fixedPaths(filepath.Join(home, ".cursor", "mcp.json")),
			Shape:        shapeCursor},
		{ID: "zed", Label: "Zed",
			DetectDirs:   fixedPaths(zedDir),
			InstallPaths: fixedPaths(filepath.Join(zedDir, "settings.json")),
			Shape:        shapeZed},
		{ID: "cline", Label: "Cline (VS Code extension / CLI)",
			DetectDirs: fixedPaths(vscodeCline, clineCLIDir),
			InstallPaths: func() []string {
				// Install into every Cline location present: the VS Code extension's
				// globalStorage and the Cline CLI's data directory (~/.cline/data, or
				// CLINE_DATA_DIR when set).
				var paths []string
				if dirExists(vscodeCline) {
					paths = append(paths, clineVSCodeSettings(vscodeCline))
				}
				if dirExists(clineCLIDir) {
					paths = append(paths, clineCLISettings(clineCLIData))
				}
				return paths
			},
			RemovePaths: fixedPaths(clineVSCodeSettings(vscodeCline), clineCLISettings(clineCLIData)),
			Shape:       shapeMcpServers},
	}, nil
}

func clineVSCodeSettings(dir string) string {
	return filepath.Join(dir, "settings", "cline_mcp_settings.json")
}

// clineCLISettings returns the Cline CLI's MCP settings file inside its data directory.
func clineCLISettings(dataDir string) string {
	return filepath.Join(dataDir, "settings", "cline_mcp_settings.json")
}

// appDataDir returns %APPDATA%, falling back to its default location under the home directory.
// Only meaningful on Windows.
func appDataDir(home string) string {
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	return filepath.Join(home, "AppData", "Roaming")
}

// xdgConfigHome returns $XDG_CONFIG_HOME, defaulting to ~/.config when it is unset or empty.
func xdgConfigHome(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	return filepath.Join(home, ".config")
}

// clineDir resolves the Cline VS Code extension's globalStorage directory for stable VS Code.
func clineDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "saoudrizwan.claude-dev"), nil
	case "windows":
		return filepath.Join(appDataDir(home), "Code", "User", "globalStorage", "saoudrizwan.claude-dev"), nil
	default:
		return filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev"), nil
	}
}

// detectTarget is generic over every target: it only consults the target's own DetectCmd and
// DetectDirs, so adding a target never requires touching this function.
func detectTarget(t target) bool {
	if t.Unsupported != "" {
		return false
	}
	if t.DetectCmd != "" {
		if _, err := lookPath(t.DetectCmd); err == nil {
			return true
		}
	}
	if t.DetectDirs != nil {
		for _, dir := range t.DetectDirs() {
			if dirExists(dir) {
				return true
			}
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// --- JSON config read/write/merge ---------------------------------------------------------------

// readConfig parses a config file. A missing file is an empty config. strict reports whether the
// file was plain JSON; when it only parses after removing comments and trailing commas (JSONC),
// strict is false and the caller must not rewrite it, since re-encoding would drop the comments.
func readConfig(path string) (m map[string]any, strict bool, err error) {
	content, ok, err := fsutil.ReadOptional(path)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return map[string]any{}, true, nil
	}
	if err := json.Unmarshal(content, &m); err == nil {
		if m == nil {
			m = map[string]any{}
		}
		return m, true, nil
	}
	m = nil
	if err := json.Unmarshal(stripJSONC(content), &m); err != nil {
		return nil, false, fmt.Errorf("cannot parse %s — invalid JSON: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, false, nil
}

func writeJSON(path string, data map[string]any) error {
	// Preserve the file's existing permissions instead of always requesting 0o644: these files
	// often hold secrets for other MCP servers, and WriteAtomic's temp-file+rename means the mode
	// passed here governs the final file.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, append(b, '\n'), mode)
}

func mergeServerEntry(m map[string]any, topKey string, entry map[string]any) map[string]any {
	sub, ok := m[topKey].(map[string]any)
	if !ok {
		sub = map[string]any{}
	}
	sub["build82"] = entry
	m[topKey] = sub
	return m
}

// serverEntry builds the build82 entry in the given shape.
func serverEntry(shape configShape, binaryPath, moodlePath string) map[string]any {
	env := map[string]any{"BUILD82_MOODLE_PATH": moodlePath}
	switch shape {
	case shapeMcpServers:
		return map[string]any{"command": binaryPath, "args": []string{}, "env": env}
	case shapeCursor:
		return map[string]any{"type": "stdio", "command": binaryPath, "args": []string{}, "env": env}
	case shapeOpenCode:
		return map[string]any{"type": "local", "command": []string{binaryPath}, "environment": env, "enabled": true}
	case shapeZed:
		return map[string]any{"command": binaryPath, "args": []string{}, "env": env}
	}
	return nil
}

// writeConfig merges a build82 entry into one config file, in the target's shape, never
// overwriting the whole file. A file with comments or trailing commas is left untouched and the
// error carries the snippet to add by hand.
func writeConfig(t target, path, binaryPath, moodlePath string) error {
	topKey := t.Shape.topKey()
	if topKey == "" {
		return fmt.Errorf("target %s has no JSON config shape", t.ID)
	}
	m, strict, err := readConfig(path)
	if err != nil {
		return err
	}
	entry := serverEntry(t.Shape, binaryPath, moodlePath)
	if !strict {
		snippet, _ := json.MarshalIndent(map[string]any{"build82": entry}, "", "  ")
		return fmt.Errorf("%s contains comments or trailing commas; it was left unchanged so they are not lost. "+
			"Add this inside its %q object manually:\n%s", path, topKey, snippet)
	}
	return writeJSON(path, mergeServerEntry(m, topKey, entry))
}

// runCLI runs one of a tool's own CLI commands, turning a failure into an error carrying the
// tool's output.
func runCLI(bin string, args []string) error {
	if out, err := runCommand(bin, args...); err != nil {
		return cliError(bin, args, out, err)
	}
	return nil
}

func cliError(bin string, args []string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s %s failed: %s", bin, strings.Join(args[:min(2, len(args))], " "), msg)
}

// removeCLIRegistration removes every build82 registration a CLI-managed tool reports, so that a
// following `mcp add` never collides with an existing one. removed reports whether anything was
// removed; warnings describe registrations that were deliberately left in place.
//
// For Claude Code, `claude mcp get` shows only the registration that takes precedence (local over
// project over user), so it is queried again after each removal. Local and user registrations are
// removed; a project registration lives in the project's shared .mcp.json and is never modified.
// Because a project registration hides a user one from `claude mcp get`, the user scope is then
// removed directly, treating Claude's "No MCP server named" answer as nothing to remove.
func removeCLIRegistration(c *cliSpec) (removed bool, warnings []string, err error) {
	if c.Scope == nil {
		if _, err := runCommand(c.Bin, c.GetArgs...); err != nil {
			return false, nil, nil
		}
		if err := runCLI(c.Bin, c.RemoveArgs("")); err != nil {
			return false, nil, err
		}
		return true, nil, nil
	}
	seen := map[string]bool{}
	for {
		out, err := runCommand(c.Bin, c.GetArgs...)
		if err != nil {
			return removed, warnings, nil
		}
		scope := c.Scope(out)
		switch {
		case scope == "":
			return removed, warnings, fmt.Errorf("could not determine the scope of the build82 registration from `%s %s`; "+
				"remove it manually with `%s mcp remove --scope <scope> build82`", c.Bin, strings.Join(c.GetArgs, " "), c.Bin)
		case scope == scopeProject:
			warnings = append(warnings, projectScopeWarning(c.Bin))
			out, err := runCommand(c.Bin, c.RemoveArgs(scopeUser)...)
			if err == nil {
				return true, warnings, nil
			}
			if strings.Contains(string(out), "No MCP server named") {
				return removed, warnings, nil
			}
			return removed, warnings, cliError(c.Bin, c.RemoveArgs(scopeUser), out, err)
		case seen[scope]:
			return removed, warnings, fmt.Errorf("build82 is still registered in the %s scope after `%s %s`",
				scope, c.Bin, strings.Join(c.RemoveArgs(scope), " "))
		}
		seen[scope] = true
		if err := runCLI(c.Bin, c.RemoveArgs(scope)); err != nil {
			return removed, warnings, err
		}
		removed = true
	}
}

// projectScopeWarning explains a project-scope registration that build82 leaves untouched. Such a
// registration belongs to the project directory build82 was run from, and inside that project it
// takes precedence over the user-scope one.
func projectScopeWarning(bin string) string {
	dir, err := os.Getwd()
	if err != nil {
		dir = "the current directory"
	}
	return fmt.Sprintf("build82 is also registered in project scope (.mcp.json, shared with the project) for %s; "+
		"it was left unchanged and, inside that project, it takes precedence over any user-scope registration. "+
		"To remove it, run from that directory: %s mcp remove --scope project build82", dir, bin)
}

// installTarget registers build82 in a target. replaced reports whether an existing build82
// registration was replaced; warnings describe registrations left in place.
func installTarget(t target, binaryPath, moodlePath string) (replaced bool, warnings []string, err error) {
	if t.Shape == shapeCLI {
		replaced, warnings, err = removeCLIRegistration(t.CLI)
		if err != nil {
			return replaced, warnings, err
		}
		return replaced, warnings, runCLI(t.CLI.Bin, t.CLI.AddArgs(binaryPath, moodlePath))
	}
	var paths []string
	if t.InstallPaths != nil {
		paths = t.InstallPaths()
	}
	if len(paths) == 0 {
		return false, nil, fmt.Errorf("no configuration location found for %s", t.Label)
	}
	var errs []error
	for _, p := range paths {
		replaced = replaced || fileHasEntry(p, t.Shape)
		if err := writeConfig(t, p, binaryPath, moodlePath); err != nil {
			errs = append(errs, err)
		}
	}
	return replaced, nil, errors.Join(errs...)
}

// reportInstall prints the outcome of one target's install.
func reportInstall(t target, replaced bool, warnings []string, err error) {
	switch {
	case err != nil:
		fmt.Printf("%s... failed: %v\n", t.Label, err)
	case replaced:
		fmt.Printf("%s... updated.\n", t.Label)
	default:
		fmt.Printf("%s... configured.\n", t.Label)
	}
	printWarnings(warnings)
}

func printWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Printf("  Warning: %s\n", w)
	}
}

// --- interactive prompts -------------------------------------------------------------------------

func promptMoodlePath(in *bufio.Reader) string {
	cwd, _ := os.Getwd()
	if extractors.IsMoodleRoot(cwd) {
		fmt.Printf("Detected a Moodle installation at %s. Correct? [Y/n] ", cwd)
		answer := readLine(in)
		if !strings.EqualFold(strings.TrimSpace(answer), "n") {
			return cwd
		}
	}
	for {
		fmt.Print("Moodle root path: ")
		answer := strings.TrimSpace(readLine(in))
		if answer == "" {
			answer = cwd
		}
		abs, err := filepath.Abs(answer)
		if err == nil && extractors.IsMoodleRoot(abs) {
			return abs
		}
		fmt.Printf("%q does not look like a Moodle root (expected version.php, lib/, and config.php or config-dist.php). Try again.\n", answer)
	}
}

func confirm(in *bufio.Reader, prompt string) bool {
	fmt.Print(prompt)
	answer := strings.ToLower(strings.TrimSpace(readLine(in)))
	return answer == "y"
}

func readLine(in *bufio.Reader) string {
	line, _ := in.ReadString('\n')
	return line
}

// --- top-level flow -------------------------------------------------------------------------------

func targetByID(id string) (target, bool, error) {
	ts, err := targets()
	if err != nil {
		return target{}, false, err
	}
	for _, t := range ts {
		if t.ID == id {
			return t, true, nil
		}
	}
	return target{}, false, nil
}

func supportedIDs() ([]string, error) {
	ts, err := targets()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	return ids, nil
}

// Run is the top-level `build82 install [target]` flow.
func Run(targetID string) error {
	binaryPath, err := binpath.Resolve()
	if err != nil {
		return fmt.Errorf("failed to resolve the running binary's path: %w", err)
	}

	in := bufio.NewReader(os.Stdin)

	if targetID != "" {
		t, ok, err := targetByID(targetID)
		if err != nil {
			return err
		}
		if !ok {
			ids, err := supportedIDs()
			if err != nil {
				return err
			}
			return fmt.Errorf("unknown target %q — supported targets: %s", targetID, strings.Join(ids, ", "))
		}
		if t.Unsupported != "" {
			fmt.Printf("Skipped: %s (%s).\n", t.Label, t.Unsupported)
			return nil
		}
		if !detectTarget(t) {
			fmt.Printf("Skipped: %s not detected.\n", t.Label)
			return nil
		}
		moodlePath := promptMoodlePath(in)
		replaced, warnings, err := installTarget(t, binaryPath, moodlePath)
		reportInstall(t, replaced, warnings, err)
		return err
	}

	ts, err := targets()
	if err != nil {
		return err
	}
	var detected []target
	for _, t := range ts {
		if detectTarget(t) {
			detected = append(detected, t)
		}
	}
	if len(detected) == 0 {
		ids, err := supportedIDs()
		if err != nil {
			return err
		}
		fmt.Printf("No supported AI tools detected. Supported targets: %s\n", strings.Join(ids, ", "))
		return nil
	}

	fmt.Println("Detected the following AI tools:")
	for _, t := range detected {
		fmt.Printf("  - %s\n", t.Label)
	}

	moodlePath := promptMoodlePath(in)
	if !confirm(in, fmt.Sprintf("Install build82 into all %d detected tool(s)? [y/N] ", len(detected))) {
		return nil
	}

	for _, t := range detected {
		replaced, warnings, err := installTarget(t, binaryPath, moodlePath)
		reportInstall(t, replaced, warnings, err)
	}
	return nil
}
