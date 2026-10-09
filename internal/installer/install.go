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

package installer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/prompt"
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

// configShape identifies the layout of a tool's MCP configuration, which determines the JSON key and
// entry fields written for build82 (or that the tool is configured through its own CLI).
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
	// Bin is the CLI executable name, looked up on PATH.
	Bin string
	// AddArgs returns the command that registers build82 for the given binary and Moodle path.
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

// target is one entry in the install table: an AI tool build82 can be registered in, with how to
// detect it and where or how its configuration is written.
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

// removePaths returns the config files an uninstall inspects: RemovePaths when set, otherwise
// InstallPaths, otherwise nil.
func (t target) removePaths() []string {
	if t.RemovePaths != nil {
		return t.RemovePaths()
	}
	if t.InstallPaths != nil {
		return t.InstallPaths()
	}
	return nil
}

// fixedPaths returns a path-list function that always yields the given paths.
func fixedPaths(paths ...string) func() []string {
	return func() []string { return paths }
}

// targets builds the install table for the current operating system. It returns an error when the
// home directory cannot be resolved (unset $HOME/%USERPROFILE%): defaulting to "" would make every
// path relative to the working directory, so install/uninstall could touch an unrelated file.
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
	var claudeDesktopDir string
	switch goos {
	case "darwin":
		claudeDesktopDir = filepath.Join(home, "Library", "Application Support", "Claude")
	case "windows":
		claudeDesktopDir = filepath.Join(appData, "Claude")
	case "linux":
		claudeDesktopDir = filepath.Join(xdgConfigHome(home), "Claude")
	default:
		claudeDesktop.Unsupported = "Claude Desktop is only available for macOS, Windows and Linux"
	}
	if claudeDesktopDir != "" {
		// On Windows the MSIX package (the claude.ai download and the Microsoft Store) reads a
		// virtualized copy of the directory, so those directories are looked at as well.
		dirs := func() []string { return append([]string{claudeDesktopDir}, claudeDesktopMSIXDirs(home)...) }
		claudeDesktop.DetectDirs = dirs
		claudeDesktop.InstallPaths = func() []string {
			var paths []string
			for _, dir := range dirs() {
				if dirExists(dir) {
					paths = append(paths, filepath.Join(dir, "claude_desktop_config.json"))
				}
			}
			if len(paths) == 0 {
				paths = append(paths, filepath.Join(claudeDesktopDir, "claude_desktop_config.json"))
			}
			return paths
		}
		claudeDesktop.RemovePaths = func() []string {
			var paths []string
			for _, dir := range dirs() {
				paths = append(paths, filepath.Join(dir, "claude_desktop_config.json"))
			}
			return paths
		}
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

	clineCLIDir, clineCLIData := clineCLIDirs(home)

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

// clineVSCodeSettings returns the MCP settings file of the Cline VS Code extension inside its
// globalStorage directory `dir`.
func clineVSCodeSettings(dir string) string {
	return filepath.Join(dir, "settings", "cline_mcp_settings.json")
}

// clineCLISettings returns the Cline CLI's MCP settings file inside its data directory.
func clineCLISettings(dataDir string) string {
	return filepath.Join(dataDir, "settings", "cline_mcp_settings.json")
}

// clineCLIDirs returns the directory whose presence marks a Cline CLI install and the data
// directory holding its settings: ~/.cline and ~/.cline/data by default. CLINE_DIR, when it is an
// absolute path, replaces ~/.cline (and so moves the data directory under it); CLINE_DATA_DIR,
// when set, replaces the data directory as a whole and is then used as the marker too.
func clineCLIDirs(home string) (marker, data string) {
	if v := os.Getenv("CLINE_DATA_DIR"); v != "" {
		return v, v
	}
	marker = filepath.Join(home, ".cline")
	if v := os.Getenv("CLINE_DIR"); filepath.IsAbs(v) {
		marker = v
	}
	return marker, filepath.Join(marker, "data")
}

// claudeDesktopMSIXDirs returns the Claude directories of Claude Desktop's MSIX package on
// Windows: the app's AppData is virtualized under
// %LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude, and that copy of
// claude_desktop_config.json is the one the app reads. It returns nil on other systems or when no
// such package directory exists.
func claudeDesktopMSIXDirs(home string) []string {
	if goos != "windows" {
		return nil
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(home, "AppData", "Local")
	}
	packages := filepath.Join(local, "Packages")
	entries, err := os.ReadDir(packages)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "Claude_") {
			dirs = append(dirs, filepath.Join(packages, e.Name(), "LocalCache", "Roaming", "Claude"))
		}
	}
	return dirs
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

// detectTarget reports whether the tool behind `t` appears to be installed: its DetectCmd is on
// PATH or one of its DetectDirs exists. Unsupported targets are never detected.
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

// dirExists reports whether `path` exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileExists reports whether `path` exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// --- JSON config read/write/merge ---------------------------------------------------------------

// errUnparsableConfig reports a config file whose content is not a JSON object, even after
// removing comments and trailing commas.
var errUnparsableConfig = errors.New("not a JSON object")

// manualEditError reports a config file left unchanged because rewriting it would lose the user's
// comments or trailing commas; its message says what to change by hand.
type manualEditError struct{ msg string }

// Error returns the instructions for the manual edit.
func (e *manualEditError) Error() string { return e.msg }

// onlyManual reports whether `err` is non-nil and every error it holds (through errors.Join) is a
// *manualEditError, so the target needs a manual step but nothing actually failed.
func onlyManual(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if !onlyManual(e) {
				return false
			}
		}
		return true
	}
	var manual *manualEditError
	return errors.As(err, &manual)
}

// readConfig parses the JSON object in the config file at `path`, as loadConfig does.
func readConfig(path string) (m map[string]any, strict bool, err error) {
	m, strict, _, err = loadConfig(path)
	return m, strict, err
}

// loadConfig reads the config file at `path` and returns its JSON object, with numbers kept as
// json.Number, and its content without a UTF-8 byte order mark (nil for a missing file) for a
// rewrite that keeps it as written. A missing or blank file yields an empty object. strict reports
// whether the file was plain JSON; when it only parses after removing comments and trailing commas
// (JSONC), strict is false and the caller must not rewrite it, since re-encoding would drop the
// comments. It returns an error when the file cannot be read or is not a JSON(C) object.
func loadConfig(path string) (m map[string]any, strict bool, raw []byte, err error) {
	raw, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, true, nil, nil
	}
	if err != nil {
		return nil, false, nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, true, raw, nil
	}
	if m, ok := decodeObject(raw); ok {
		return m, true, raw, nil
	}
	if m, ok := decodeObject(stripJSONC(raw)); ok {
		return m, false, raw, nil
	}
	return nil, false, nil, fmt.Errorf("cannot parse %s: %w", path, errUnparsableConfig)
}

// decodeObject decodes `raw` as exactly one JSON object, keeping numbers as json.Number. Anything
// after the object other than white space makes it fail.
func decodeObject(raw []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return m, true
}

// jsonMember is one key of a JSON object and its value as written.
type jsonMember struct {
	key   string
	value json.RawMessage
}

// objectMembers decodes `raw`, a JSON object, into its members in the order they are written, each
// value compacted. A key written more than once keeps its first position and its last value. Blank
// `raw` has no members. It returns an error when `raw` is not a single JSON object.
func objectMembers(raw []byte) ([]jsonMember, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errUnparsableConfig
	}
	var members []jsonMember
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errUnparsableConfig
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return nil, err
		}
		members = setMember(members, key, compact.Bytes())
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return members, nil
}

// memberValue returns the value of `key` in `members`, and whether it is there.
func memberValue(members []jsonMember, key string) (json.RawMessage, bool) {
	for _, m := range members {
		if m.key == key {
			return m.value, true
		}
	}
	return nil, false
}

// setMember sets `key` to `value` in `members`: in place when the key is there, otherwise appended
// at the end. It returns the updated members.
func setMember(members []jsonMember, key string, value json.RawMessage) []jsonMember {
	for i := range members {
		if members[i].key == key {
			members[i].value = value
			return members
		}
	}
	return append(members, jsonMember{key: key, value: value})
}

// deleteMember returns `members` without `key`.
func deleteMember(members []jsonMember, key string) []jsonMember {
	out := members[:0]
	for _, m := range members {
		if m.key != key {
			out = append(out, m)
		}
	}
	return out
}

// encodeMembers returns `members` as one compact JSON object, in order, each value written as held.
func encodeMembers(members []jsonMember) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := marshalNoEscape(m.key) // a string always encodes
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(m.value)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// marshalNoEscape encodes `v` as compact JSON without escaping "&", "<" and ">", so values such as
// URLs and commands keep their form when a file is rewritten.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// editServers rewrites the config file at `path`, whose content is `raw` (a JSON object, or blank),
// after passing the members of the object under `topKey` through `edit`; a missing or null object
// counts as empty, and a missing one is added at the end. Every other key and value stays as in
// `raw`, in the same order. The file is written indented with two spaces and a trailing newline,
// keeping the mode of an existing file (0o644 for a new one); when `path` is a symbolic link, the
// file it points to is written and the link is kept.
func editServers(path string, raw []byte, topKey string, edit func([]jsonMember) []jsonMember) error {
	top, err := objectMembers(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var servers []jsonMember
	if v, ok := memberValue(top, topKey); ok && !bytes.Equal(v, []byte("null")) {
		if servers, err = objectMembers(v); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	top = setMember(top, topKey, encodeMembers(edit(servers)))
	var out bytes.Buffer
	if err := json.Indent(&out, encodeMembers(top), "", "  "); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out.WriteByte('\n')

	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	// These files often hold secrets for other MCP servers, and WriteAtomic's temp-file+rename
	// means the mode passed here governs the final file, so the existing mode is carried over.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return fsutil.WriteAtomic(path, out.Bytes(), mode)
}

// serversObject returns the object under `topKey` in `m`, an empty one when it is missing or null.
// It returns an error when the key holds anything else, since replacing it would drop what the
// user keeps there.
func serversObject(m map[string]any, topKey, path string) (map[string]any, error) {
	v, ok := m[topKey]
	if !ok || v == nil {
		return map[string]any{}, nil
	}
	sub, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: %q is not a JSON object, so the file was left unchanged; fix it, then try again", path, topKey)
	}
	return sub, nil
}

// entryMatches reports whether `current`, an existing build82 entry, holds every key of `want`
// with an equal JSON value. Keys the user added to the entry are allowed.
func entryMatches(current any, want map[string]any) bool {
	cur, ok := current.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range want {
		a, errA := json.Marshal(cur[k])
		b, errB := json.Marshal(v)
		if errA != nil || errB != nil || !bytes.Equal(a, b) {
			return false
		}
	}
	return true
}

// serverEntry builds the build82 entry for the given shape, launching `binaryPath` with
// BUILD82_MOODLE_PATH set to `moodlePath`. It returns nil for shapes with no JSON entry.
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

// writeConfig merges a build82 entry into the config file at `path`, in the shape of target `t`,
// preserving every other key, their order and their values as written. A file that already holds
// the wanted entry is left unchanged. Otherwise a file with comments or trailing commas is left
// untouched and the returned *manualEditError carries the snippet to add by hand. It also fails
// when the file cannot be read, parsed or written, or the shape's key holds a non-object value.
func writeConfig(t target, path, binaryPath, moodlePath string) error {
	topKey := t.Shape.topKey()
	if topKey == "" {
		return fmt.Errorf("target %s has no JSON config shape", t.ID)
	}
	m, strict, raw, err := loadConfig(path)
	if err != nil {
		return err
	}
	servers, err := serversObject(m, topKey, path)
	if err != nil {
		return err
	}
	entry := serverEntry(t.Shape, binaryPath, moodlePath)
	if current, ok := servers["build82"]; ok && entryMatches(current, entry) {
		return nil
	}
	if !strict {
		snippet, _ := json.MarshalIndent(map[string]any{"build82": entry}, "", "  ")
		return &manualEditError{fmt.Sprintf("%s contains comments or trailing commas; it was left unchanged so they are not lost. "+
			"Add this inside its %q object manually:\n%s", path, topKey, snippet)}
	}
	value, err := marshalNoEscape(entry)
	if err != nil {
		return err
	}
	return editServers(path, raw, topKey, func(members []jsonMember) []jsonMember {
		return setMember(members, "build82", value)
	})
}

// runCLI runs one of a tool's own CLI commands, turning a failure into an error carrying the
// tool's output.
func runCLI(bin string, args []string) error {
	if out, err := runCommand(bin, args...); err != nil {
		return cliError(bin, args, out, err)
	}
	return nil
}

// cliError builds the error for a failed tool CLI command from its output, falling back to `err`
// when the output is empty.
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
// For a tool with a single configuration (Codex), a failed `mcp get` means "not registered" only
// when its output says so (notRegisteredOutput); any other failure is returned as an error.
//
// For Claude Code, `claude mcp get` shows only the registration that takes precedence (local over
// project over user), so it is queried again after each removal. Local and user registrations are
// removed; a project registration lives in the project's shared .mcp.json and is never modified.
// Because a project registration hides a user one from `claude mcp get`, the user scope is then
// removed directly, treating Claude's "No MCP server named" answer as nothing to remove.
func removeCLIRegistration(c *cliSpec) (removed bool, warnings []string, err error) {
	if c.Scope == nil {
		if out, err := runCommand(c.Bin, c.GetArgs...); err != nil {
			if notRegisteredOutput(out) {
				return false, nil, nil
			}
			return false, nil, cliError(c.Bin, c.GetArgs, out, err)
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
			if notRegisteredOutput(out) {
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

// notRegisteredOutput reports whether `out`, the output of a failed `mcp get` or `mcp remove`, says
// that no server of that name is registered ("No MCP server named ..."), the answer of both the
// claude and the codex CLI.
func notRegisteredOutput(out []byte) bool {
	return strings.Contains(string(out), "No MCP server named")
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
		has, _ := fileHasEntry(p, t.Shape) // an unreadable file makes writeConfig fail below
		replaced = replaced || has
		if err := writeConfig(t, p, binaryPath, moodlePath); err != nil {
			errs = append(errs, err)
		}
	}
	return replaced, nil, errors.Join(errs...)
}

// reportInstall prints the outcome of one target's install: "manual step needed" when the only
// problems are files left for the user to edit, "failed" for any other error.
func reportInstall(t target, replaced bool, warnings []string, err error) {
	switch {
	case onlyManual(err):
		fmt.Printf("%s... manual step needed: %v\n", t.Label, err)
	case err != nil:
		fmt.Printf("%s... failed: %v\n", t.Label, err)
	case replaced:
		fmt.Printf("%s... updated.\n", t.Label)
	default:
		fmt.Printf("%s... configured.\n", t.Label)
	}
	printWarnings(warnings)
}

// printWarnings prints each warning as an indented "Warning:" line.
func printWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Printf("  Warning: %s\n", w)
	}
}

// --- interactive prompts -------------------------------------------------------------------------

// promptMoodlePath asks for the Moodle root, offering the working directory when it is one, and
// repeats until the answer is a valid Moodle root. An empty answer selects the working directory.
// It returns the absolute path, or an error when the input ends before a valid root is given or
// `ctx` ends while waiting for an answer (wrapping prompt.ErrInterrupted).
func promptMoodlePath(ctx context.Context, in *bufio.Reader) (string, error) {
	cwd, _ := os.Getwd()
	if extractors.IsMoodleRoot(cwd) {
		fmt.Printf("Detected a Moodle installation at %s. Correct? [Y/n] ", cwd)
		answer, _, err := prompt.ReadLine(ctx, in)
		if err != nil {
			fmt.Println()
			return "", err
		}
		if !strings.EqualFold(strings.TrimSpace(answer), "n") {
			return cwd, nil
		}
	}
	for {
		fmt.Print("Moodle root path: ")
		line, eof, err := prompt.ReadLine(ctx, in)
		if err != nil {
			fmt.Println()
			return "", err
		}
		answer := strings.TrimSpace(line)
		if eof && answer == "" {
			return "", errors.New("no Moodle path provided (stdin closed)")
		}
		if answer == "" {
			answer = cwd
		}
		abs, err := filepath.Abs(answer)
		if err == nil && extractors.IsMoodleRoot(abs) {
			return abs, nil
		}
		if eof {
			return "", fmt.Errorf("%q is not a Moodle root and stdin is closed", answer)
		}
		fmt.Printf("%q does not look like a Moodle root (expected version.php, lib/, and config.php or config-dist.php). Try again.\n", answer)
	}
}

// confirm prints `question` and reports whether the answer read from `in` is "y"
// (case-insensitive). It returns an error wrapping prompt.ErrInterrupted when `ctx` ends first.
func confirm(ctx context.Context, in *bufio.Reader, question string) (bool, error) {
	return prompt.Confirm(ctx, in, os.Stdout, question)
}

// --- top-level flow -------------------------------------------------------------------------------

// targetByID looks up an install target by its ID. found is false when no target has that ID; the
// error is non-nil only when the target table cannot be built.
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

// supportedIDs returns the IDs of all install targets, in table order.
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

// Run is the top-level `build82 install [target]` flow. With a non-empty `targetID` it installs
// into that target only (skipping it when unsupported or not detected); otherwise it lists the
// detected tools and installs into all of them after confirmation. It returns an error for an
// unknown target, an unresolvable binary path or home directory, any target whose install failed
// or needs a manual step, or `ctx` ending — at a prompt (wrapping prompt.ErrInterrupted, nothing
// changed) or between two targets.
func Run(ctx context.Context, targetID string) error {
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
		moodlePath, err := promptMoodlePath(ctx, in)
		if err != nil {
			return err
		}
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

	moodlePath, err := promptMoodlePath(ctx, in)
	if err != nil {
		return err
	}
	ok, err := confirm(ctx, in, fmt.Sprintf("Install build82 into all %d detected tool(s)? [y/N] ", len(detected)))
	if err != nil || !ok {
		return err
	}

	problems := 0
	for i, t := range detected {
		if ctx.Err() != nil {
			return fmt.Errorf("interrupted after %d of %d tool(s): %w", i, len(detected), ctx.Err())
		}
		replaced, warnings, err := installTarget(t, binaryPath, moodlePath)
		reportInstall(t, replaced, warnings, err)
		if err != nil {
			problems++
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d of %d tool(s) failed or need a manual step; see above", problems, len(detected))
	}
	return nil
}
