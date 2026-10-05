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

package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestWithRecoverResource_ConvertsPanicToErrorResult verifies that withRecoverResource converts a
// panic inside a resource (or resource template) handler — e.g. extractors.DetectPlugin or a
// filepath.WalkDir hitting a malformed/adversarial plugin directory — into a normal error return
// instead of letting the panic escape and crash the process.
func TestWithRecoverResource_ConvertsPanicToErrorResult(t *testing.T) {
	panicky := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		panic("boom: simulated resource handler failure")
	}
	wrapped := withRecoverResource(panicky)

	result, err := wrapped(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "moodle://context"},
	})
	if err == nil {
		t.Fatal("expected the panic to be converted into a returned error")
	}
	if result != nil {
		t.Errorf("expected a nil result alongside the error, got: %+v", result)
	}
	if !strings.Contains(err.Error(), "boom: simulated resource handler failure") {
		t.Errorf("expected the panic message in the error, got: %v", err)
	}
}

// TestWithRecoverResource_PassesThroughNormalResults confirms the wrapper is transparent when the
// handler doesn't panic — it must return exactly what the handler returned, not swallow or alter it.
func TestWithRecoverResource_PassesThroughNormalResults(t *testing.T) {
	normal := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: "all good"}},
		}, nil
	}
	wrapped := withRecoverResource(normal)

	result, err := wrapped(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "moodle://context"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text != "all good" {
		t.Errorf("expected the handler's own result to pass through unchanged, got: %+v", result)
	}
}
