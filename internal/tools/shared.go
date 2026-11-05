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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/resources"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// textResult builds a single-text-block tool response with the given `text`, flagged as an error
// when `isError` is true.
func textResult(isError bool, text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: isError,
	}
}

// withRecover wraps the tool handler `fn` so that a panic anywhere inside it (most likely deep in
// an extractor or generator fed malformed plugin source) is converted into an IsError text result
// instead of propagating. The MCP SDK's tool dispatch does not recover panics, so an unrecovered
// one would terminate the whole server process for every connected client. Every registered tool
// is wrapped with it. The panic is returned as an error, so the result carries no structured
// content.
func withRecover[In, Out any](
	fn func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (result *mcp.CallToolResult, out Out, err error) {
		defer func() {
			if r := recover(); r != nil {
				var zero Out
				result, out, err = nil, zero, fmt.Errorf("❌ Internal error while handling this request: %v", r)
			}
		}()
		return fn(ctx, req, in)
	}
}

// toolHandler is the signature of a typed tool handler.
type toolHandler[In, Out any] func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)

// notifyResourcesUpdated sends notifications/resources/updated for each of `uris` to the sessions
// subscribed to it. It uses a background context, so it also works from the watcher's
// asynchronous callback after the request that started the watcher ended, and it is best-effort:
// a session that cannot be reached is skipped. When any URI is given, it first drops the cached
// moodle://plugins/with-context listing (resources.InvalidatePluginsWithContext).
func notifyResourcesUpdated(server *mcp.Server, uris []string) {
	if len(uris) > 0 {
		// A client that re-reads an announced resource at once must not get the listing cached
		// before the regeneration.
		resources.InvalidatePluginsWithContext()
	}
	for _, uri := range uris {
		_ = server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri})
	}
}

// withResourceNotifications wraps the handler `fn` of a tool that rewrites generated files: after
// a call that succeeded (no error, and a result not flagged IsError), the resources named by
// urisOf(in, out) are announced as updated to their subscribers. A nil or empty list notifies
// nothing.
func withResourceNotifications[In, Out any](server *mcp.Server, fn toolHandler[In, Out], urisOf func(In, Out) []string) toolHandler[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		res, out, err := fn(ctx, req, in)
		if err == nil && (res == nil || !res.IsError) {
			notifyResourcesUpdated(server, urisOf(in, out))
		}
		return res, out, err
	}
}

// pluginResourceURIs returns the resources a regeneration of the plugins `components` changes:
// each plugin's resources and the global ones that index the plugins (resources.PluginIndexURIs).
// It returns nil when `components` is empty.
func pluginResourceURIs(components ...string) []string {
	if len(components) == 0 {
		return nil
	}
	var uris []string
	for _, c := range components {
		uris = append(uris, resources.PluginURIs(c)...)
	}
	return append(uris, resources.PluginIndexURIs()...)
}

// structuredResult returns the result of a tool with structured output `out`: a single text block
// holding `text` for the text format, or `out` as indented JSON for the JSON format, flagged as an
// error when `isError` is true. The SDK publishes `out` as the result's structuredContent in both
// formats.
func structuredResult[Out any](format Format, isError bool, text string, out Out) (*mcp.CallToolResult, Out, error) {
	if format == FormatJSON {
		return jsonResult(isError, out), out, nil
	}
	return textResult(isError, text), out, nil
}

// toolError returns an error carrying `text`. A tool handler returns it for a failure that has no
// structured output (a missing or invalid argument, a missing configuration); the SDK turns it
// into an IsError result whose single text block is `text`, without structuredContent.
func toolError(text string) error {
	return errors.New(text)
}

// resultError converts the error result `res`, made of text blocks, into a toolError carrying its
// text, so an error built as a result can be returned by a tool with structured output.
func resultError(res *mcp.CallToolResult) error {
	return toolError(textOf(res))
}

// textOf returns the text blocks of `res` joined by newlines.
func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// enumSchemas declares the values build82's enum-like input types accept, as JSON Schema enums.
// The empty string stands for the documented default.
var enumSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[Format]():              {Type: "string", Enum: []any{"", "text", "json"}},
	reflect.TypeFor[WatchAction]():         {Type: "string", Enum: []any{"", "start", "stop", "status"}},
	reflect.TypeFor[BatchMode]():           {Type: "string", Enum: []any{"", "dev", "all", "list"}},
	reflect.TypeFor[ApiVisibilityFilter](): {Type: "string", Enum: []any{"", "public", "deprecated", "all"}},
}

// inputSchemaFor returns the input schema of a tool whose input is T, inferred as the SDK does but
// with the enum-like fields restricted to their values (enumSchemas), so the SDK rejects any other
// value (e.g. "Stop" for watch_plugins' action) before the handler runs instead of the handler
// falling back to a default action. It panics when T cannot be described, which is a programming
// error caught at server start.
func inputSchemaFor[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: enumSchemas})
	if err != nil {
		panic(fmt.Sprintf("input schema for %T: %v", *new(T), err))
	}
	return schema
}

// toolAnnotations returns the annotations of a tool with every hint set explicitly, so no client
// falls back to the protocol defaults (which assume a destructive, open-world tool): `title` is the
// human-readable name; `readOnly` means the tool changes nothing; `destructive` means it may
// overwrite or delete existing data; `idempotent` means repeating a call with the same arguments
// has no further effect; `openWorld` means it reaches entities outside the Moodle installation.
func toolAnnotations(title string, readOnly, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   &openWorld,
	}
}

// configErrorText returns the message for a configuration that cannot be resolved because of
// `err`.
func configErrorText(err error) string {
	return "❌ Failed to resolve build82 configuration: " + err.Error()
}

// configError returns the toolError for a configuration that cannot be resolved because of `err`.
func configError(err error) error {
	return toolError(configErrorText(err))
}

// Format is the optional output-format field shared by every tool's input struct.
// "text" (the zero value) renders Markdown/plain text; "json" renders the same underlying
// data as a JSON string in that same single text block.
type Format string

// Supported values of Format.
const (
	FormatText Format = ""
	FormatJSON Format = "json"
)

// jsonResult marshals `data` as indented JSON into a single text block flagged as an error when
// `isError` is true. If marshaling fails, it returns an error result describing the failure.
func jsonResult(isError bool, data any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return textResult(true, "❌ Failed to encode JSON response: "+err.Error())
	}
	return textResult(isError, string(b))
}

// FailedFile is the JSON shape of one file that failed to generate, used by every tool whose
// output distinguishes generated, skipped and failed generator results.
type FailedFile struct {
	File  string `json:"file" jsonschema:"Path of the file that failed to generate (relative to the Moodle root for global files, to the plugin for plugin files)"`
	Error string `json:"error" jsonschema:"Why the file could not be generated"`
}

// classifyResults splits generator `results` into the names of generated files, the names of
// skipped (cached) files, and the records of failed files. Every file name is made relative to
// `base` (the Moodle root for global generators, the plugin's own path for per-plugin ones) so the
// output never exposes an absolute host path.
func classifyResults(results []generators.GeneratorResult, base string) (generated, skipped []string, failed []FailedFile) {
	for _, r := range results {
		rel := relativeToMoodle(base, r.File)
		switch {
		case !r.Success:
			failed = append(failed, FailedFile{File: rel, Error: r.Error})
		case r.Skipped:
			skipped = append(skipped, rel)
		default:
			generated = append(generated, rel)
		}
	}
	return
}

// writeGenerationResults writes the "## Results" section of a generation report to `b`: the counts,
// the `failed` files with their errors, then the `generated` and `skipped` (cached) file names,
// each list sorted. The slices themselves are not reordered.
func writeGenerationResults(b *strings.Builder, generated, skipped []string, failed []FailedFile) {
	fmt.Fprintf(b, "## Results\n\n%d generated, %d cached, %d failed\n\n", len(generated), len(skipped), len(failed))
	if len(failed) > 0 {
		b.WriteString("### Failed\n\n")
		for _, f := range failed {
			fmt.Fprintf(b, "✖ %s: %s\n", f.File, f.Error)
		}
		b.WriteString("\n")
	}
	b.WriteString("### Generated\n\n")
	for _, n := range slices.Sorted(slices.Values(generated)) {
		fmt.Fprintf(b, "✔ %s\n", n)
	}
	if len(skipped) > 0 {
		b.WriteString("\n### Cached\n\n")
		for _, n := range slices.Sorted(slices.Values(skipped)) {
			fmt.Fprintf(b, "✔ %s\n", n)
		}
	}
}

// resolvedPlugin is the outcome of resolving and validating a plugin identifier: the absolute
// plugin directory and its detected metadata.
type resolvedPlugin struct {
	Path string
	Info extractors.PluginInfo
}

// resolveAndValidatePlugin resolves `identifier` — a frankenstyle component ("local_myplugin"), a
// path relative to the Moodle root ("local/myplugin"), or an absolute path — against `moodlePath`,
// and checks that it lies within the Moodle root, is not the Moodle root itself (whose version.php
// is core's, and whose ContextDir holds the global files), exists, and contains a version.php. On any
// failure it returns a non-nil error result (IsError set) that the caller should return as is; on
// success the result is nil and the resolvedPlugin is populated.
func resolveAndValidatePlugin(identifier, moodlePath string) (resolvedPlugin, *mcp.CallToolResult) {
	full, _ := filepath.Abs(resolvePluginIdentifier(identifier, moodlePath))

	// The messages below never embed moodlePath or the absolute `full` path: absolute host paths
	// reveal the directory structure (often an OS username), so any path shown is first made
	// relative to moodlePath via relativeToMoodle.
	if !moodletype.IsWithinMoodle(full, moodlePath) {
		return resolvedPlugin{}, textResult(true,
			"❌ Invalid plugin path: must be within the Moodle installation.")
	}
	if moodletype.IsRoot(full, moodlePath) {
		return resolvedPlugin{}, textResult(true,
			"❌ Invalid plugin path: the Moodle root itself is not a plugin; name a plugin directory inside it.")
	}
	if _, err := os.Stat(full); err != nil {
		return resolvedPlugin{}, textResult(true, fmt.Sprintf(
			"❌ Plugin directory not found: %s\n\nCheck that the path is correct relative to the Moodle root.",
			relativeToMoodle(moodlePath, full)))
	}
	if _, err := os.Stat(filepath.Join(full, "version.php")); err != nil {
		return resolvedPlugin{}, textResult(true, fmt.Sprintf(
			"❌ %s does not appear to be a Moodle plugin.\n\nA plugin must have a version.php file at its root.",
			relativeToMoodle(moodlePath, full)))
	}

	info, err := extractors.DetectPlugin(full)
	if err != nil {
		return resolvedPlugin{}, textResult(true, "❌ Failed to detect plugin: "+err.Error())
	}
	info.MoodlePath = moodlePath
	return resolvedPlugin{Path: full, Info: info}, nil
}

// resolvePluginIdentifier maps `identifier` to a filesystem path without checking containment or
// existence, so callers must still run moodletype.IsWithinMoodle on the result. It first tries
// moodletype.ResolvePluginPath (component, relative, or absolute, existing paths only); when that
// finds nothing, it treats `identifier` as a path (absolute, or joined onto `moodlePath`). That
// way a directory whose name merely looks like a component (e.g. a literal "local_x" folder) still
// resolves, and a missing plugin yields a meaningful "not found" message.
func resolvePluginIdentifier(identifier, moodlePath string) string {
	if path, ok := moodletype.ResolvePluginPath(identifier, moodlePath); ok {
		return path
	}
	if filepath.IsAbs(identifier) {
		return identifier
	}
	return filepath.Join(moodlePath, identifier)
}

// resolvePluginPathWithinMoodle resolves `identifier` (component, relative path, or absolute path)
// via moodletype.ResolvePluginPath and checks that it lies within `moodlePath`
// (moodletype.IsWithinMoodle), is not the Moodle root itself, and contains a version.php. It
// returns the resolved path and true, or ("", false) when any check fails.
func resolvePluginPathWithinMoodle(identifier, moodlePath string) (string, bool) {
	path, ok := moodletype.ResolvePluginPath(identifier, moodlePath)
	if !ok || !moodletype.IsWithinMoodle(path, moodlePath) || moodletype.IsRoot(path, moodlePath) ||
		!fileExists(filepath.Join(path, "version.php")) {
		return "", false
	}
	return path, true
}

// loadConfigResult loads the build82 configuration (config.Load). It returns the configuration and
// a nil result, or a nil configuration and the error result to return as is: a configuration that
// cannot be resolved (configErrorText), or no configuration yet (toolutil.NotInitialized). A
// configuration file that exists but cannot be read is reported by config.Load on stderr and counts
// as no configuration.
func loadConfigResult() (*config.Config, *mcp.CallToolResult) {
	cfg, err := config.Load()
	if err != nil {
		return nil, textResult(true, configErrorText(err))
	}
	if cfg == nil {
		return nil, toolutil.NotInitialized()
	}
	return cfg, nil
}

// loadConfig is loadConfigResult for tools with structured output: the error result is returned
// as a toolError carrying the same text.
func loadConfig() (*config.Config, error) {
	cfg, res := loadConfigResult()
	if res != nil {
		return nil, resultError(res)
	}
	return cfg, nil
}

// readPluginFileTruncated reads the generated file `filename` from the plugin's .build82/
// directory (`pluginPath` is the plugin root), truncated to `maxChars` bytes. It returns "" when
// the file is missing or unreadable; callers treat that as "not available".
func readPluginFileTruncated(pluginPath, filename string, maxChars int) string {
	return toolutil.ReadFileTruncated(generators.PluginOutputPath(pluginPath, filename), maxChars)
}

// fileExists reports whether `path` exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether `path` exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// relativeToMoodle returns `absPath` relative to `moodlePath` with forward slashes, or `absPath`
// unchanged when no relative form exists.
func relativeToMoodle(moodlePath, absPath string) string {
	rel, err := filepath.Rel(moodlePath, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// displayPathWithinMoodle returns `absPath` relative to `moodlePath` (forward slashes) and true
// when `absPath` lies inside the Moodle root, and ("", false) otherwise, so a caller can pick its
// own fallback instead of echoing an absolute host path.
func displayPathWithinMoodle(moodlePath, absPath string) (string, bool) {
	if !moodletype.IsWithinMoodle(absPath, moodlePath) {
		return "", false
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return "", false
	}
	root, err := filepath.Abs(moodlePath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// Contained only through a symlink: the lexical relative form would climb out of the
		// root, which is both misleading and not a Moodle-relative path.
		return "", false
	}
	return filepath.ToSlash(rel), true
}
