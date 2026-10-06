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

package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestWithRecoverResource_ConvertsPanicToErrorResult verifies that a panic inside a wrapped
// handler is returned as an error with a nil result and the panic message preserved.
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

// TestWithRecoverResource_PassesThroughNormalResults verifies the wrapper returns a non-panicking
// handler's result unchanged.
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
