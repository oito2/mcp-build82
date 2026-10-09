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

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// updateGolden makes TestTools_SchemasMatchGolden rewrite the golden files instead of comparing
// against them: go test ./internal/server -run TestTools_SchemasMatchGolden -update
var updateGolden = flag.Bool("update", false, "rewrite the tool schema golden files in testdata/tools")

// structuredTools are the tools that publish an outputSchema and return structuredContent on
// success; every other tool returns text only.
var structuredTools = map[string]bool{
	"init_moodle_context": true, "generate_plugin_context": true, "plugin_batch": true,
	"update_indexes": true, "search_plugins": true, "search_api": true, "get_plugin_info": true,
	"list_dev_plugins": true, "doctor": true,
}

// listTools returns every tool the server advertises, keyed by name.
func listTools(t *testing.T, session *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

// TestTools_SchemasMatchGolden verifies that every tool's annotations, input schema and output
// schema match the golden file testdata/tools/<name>.json, so any change to a tool's contract is
// deliberate and reviewed.
func TestTools_SchemasMatchGolden(t *testing.T) {
	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	for name, tool := range listTools(t, session) {
		got, err := json.MarshalIndent(map[string]any{
			"name":         name,
			"annotations":  tool.Annotations,
			"inputSchema":  tool.InputSchema,
			"outputSchema": tool.OutputSchema,
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		path := filepath.Join("testdata", "tools", name+".json")
		if *updateGolden {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: missing golden file (run with -update): %v", name, err)
		}
		if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
			t.Errorf("%s: schema differs from %s (run with -update if intended):\n%s", name, path, got)
		}
	}
}

// TestTools_AnnotationsAreExplicit verifies that every tool sets a title and every hint
// explicitly, and that exactly the structured tools publish an output schema.
func TestTools_AnnotationsAreExplicit(t *testing.T) {
	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	tools := listTools(t, session)
	if len(tools) != 13 {
		t.Fatalf("expected 13 tools, got %d", len(tools))
	}
	for name, tool := range tools {
		a := tool.Annotations
		if a == nil || a.Title == "" || a.DestructiveHint == nil || a.OpenWorldHint == nil {
			t.Errorf("%s: annotations must set title, destructiveHint and openWorldHint explicitly, got %+v", name, a)
			continue
		}
		if *a.OpenWorldHint {
			t.Errorf("%s: no build82 tool reaches outside the Moodle installation", name)
		}
		if a.ReadOnlyHint && *a.DestructiveHint {
			t.Errorf("%s: a read-only tool cannot be destructive", name)
		}
		if hasSchema := tool.OutputSchema != nil; hasSchema != structuredTools[name] {
			t.Errorf("%s: outputSchema present = %v, want %v", name, hasSchema, structuredTools[name])
		}
	}
	for name, readOnly := range map[string]bool{"doctor": true, "search_api": true, "explain_plugin": true, "release_plugin": false, "create_plugin_skeleton": false, "generate_plugin_context": false} {
		if tools[name].Annotations.ReadOnlyHint != readOnly {
			t.Errorf("%s: readOnlyHint = %v, want %v", name, tools[name].Annotations.ReadOnlyHint, readOnly)
		}
	}
	if !*tools["release_plugin"].Annotations.DestructiveHint {
		t.Error("release_plugin replaces an existing ZIP, so it must be marked destructive")
	}
}

// resolvedOutputSchemas resolves the output schema of every structured tool for validation.
func resolvedOutputSchemas(t *testing.T, tools map[string]*mcp.Tool) map[string]*jsonschema.Resolved {
	t.Helper()
	out := map[string]*jsonschema.Resolved{}
	for name, tool := range tools {
		if tool.OutputSchema == nil {
			continue
		}
		raw, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: output schema: %v", name, err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatalf("%s: resolve output schema: %v", name, err)
		}
		out[name] = resolved
	}
	return out
}

// TestTools_StructuredContentMatchesSchema calls every structured tool in the text and JSON
// formats against the multi-plugin fixture and verifies that each successful result carries
// structuredContent that validates against the tool's output schema, and that the JSON format's
// text block holds the same document.
func TestTools_StructuredContentMatchesSchema(t *testing.T) {
	withIsolatedHome(t)
	root := buildE2EFixture(t)
	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	schemas := resolvedOutputSchemas(t, listTools(t, session))

	calls := []struct {
		name string
		args map[string]any
	}{
		{"init_moodle_context", map[string]any{"moodle_path": root}},
		{"generate_plugin_context", map[string]any{"plugin_path": "local/demo"}},
		{"plugin_batch", map[string]any{"mode": "dev"}},
		{"update_indexes", map[string]any{"include_plugins": true}},
		{"search_plugins", map[string]any{"query": "demo"}},
		{"search_plugins", map[string]any{"query": "zzzz-no-such-plugin"}},
		{"search_api", map[string]any{"query": "get"}},
		{"get_plugin_info", map[string]any{"plugin": "local_demo"}},
		{"list_dev_plugins", map[string]any{}},
		{"doctor", map[string]any{}},
	}
	for _, format := range []string{"text", "json"} {
		for _, c := range calls {
			args := map[string]any{"format": format}
			if format == "text" {
				args = map[string]any{}
			}
			for k, v := range c.args {
				args[k] = v
			}
			res := callTool(t, session, c.name, args)
			if res.IsError && c.name != "doctor" {
				t.Errorf("%s (%s): unexpected error: %s", c.name, format, toolText(t, res))
				continue
			}
			if res.StructuredContent == nil {
				t.Errorf("%s (%s): missing structuredContent", c.name, format)
				continue
			}
			if err := schemas[c.name].Validate(res.StructuredContent); err != nil {
				t.Errorf("%s (%s): structuredContent does not match the output schema: %v", c.name, format, err)
			}
			if format == "json" {
				var text any
				if err := json.Unmarshal([]byte(toolText(t, res)), &text); err != nil {
					t.Errorf("%s: JSON format text is not JSON: %v", c.name, err)
					continue
				}
				a, _ := json.Marshal(text)
				b, _ := json.Marshal(res.StructuredContent)
				if !bytes.Equal(a, b) {
					t.Errorf("%s: JSON text and structuredContent differ:\n%s\n%s", c.name, a, b)
				}
			}
		}
	}
}

// TestTools_ErrorsAndTextToolsCarryNoStructuredContent verifies that a failed call of a structured
// tool is an error without structuredContent, and that the text-only tools never return any.
func TestTools_ErrorsAndTextToolsCarryNoStructuredContent(t *testing.T) {
	withIsolatedHome(t)
	_, session, cleanup := connectInMemory(t)
	defer cleanup()

	for name, args := range map[string]map[string]any{
		"search_plugins":      {"query": "demo"},
		"list_dev_plugins":    {"format": "json"},
		"init_moodle_context": {"moodle_path": "/definitely/not/moodle"},
	} {
		res := callTool(t, session, name, args)
		if !res.IsError || res.StructuredContent != nil {
			t.Errorf("%s: expected an error without structuredContent, got isError=%v structured=%v", name, res.IsError, res.StructuredContent)
		}
	}

	root := copyFixtureMoodleTree(t)
	if res := callTool(t, session, "init_moodle_context", map[string]any{"moodle_path": root}); res.IsError {
		t.Fatalf("init: %s", toolText(t, res))
	}
	if res := callTool(t, session, "get_plugin_info", map[string]any{"plugin": "local_nosuchplugin"}); !res.IsError || res.StructuredContent != nil {
		t.Errorf("get_plugin_info on a missing plugin: expected an error without structuredContent, got %+v", res)
	}
	for name, args := range map[string]map[string]any{
		"explain_plugin": {"plugin": "local_demo"},
		"watch_plugins":  {"action": "status"},
	} {
		res := callTool(t, session, name, args)
		if res.StructuredContent != nil {
			t.Errorf("%s: text-only tool returned structuredContent %v", name, res.StructuredContent)
		}
	}
}
