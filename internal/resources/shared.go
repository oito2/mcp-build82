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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withRecoverResource wraps a resource (or resource template) handler so a panic anywhere inside
// it (most likely deep in extractors.DetectPlugin or a filepath.WalkDir over a malformed/adversarial
// plugin directory) becomes a normal error return instead of an unrecovered panic. The SDK's
// readResource dispatch does not recover from a panicking ResourceHandler, so an unrecovered panic
// would kill the whole server process for every connected client/session. mcp.ResourceHandler is
// the same function type used by both server.AddResource and server.AddResourceTemplate, so this
// one wrapper covers every registration in this package.
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
