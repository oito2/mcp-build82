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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withRecoverResource wraps the resource or resource-template handler `fn` so that a panic inside
// it (most likely in extractors.DetectPlugin or a directory walk over a malformed plugin
// directory) is returned as an error with a nil result instead of propagating. The SDK does not
// recover from a panicking handler, so an unrecovered panic would terminate the server for every
// connected session. Results and errors from a non-panicking `fn` pass through unchanged.
func withRecoverResource(fn mcp.ResourceHandler) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (result *mcp.ReadResourceResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = nil
				err = fmt.Errorf("internal error while reading this resource: %v", r)
			}
		}()
		return fn(ctx, req)
	}
}
