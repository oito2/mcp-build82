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

// Package binpath resolves the path to the currently running build82 binary. Shared by
// internal/installer and internal/selfupdate, which each need this to locate/replace the running
// executable, using os.Executable followed by filepath.EvalSymlinks.
package binpath

import (
	"fmt"
	"os"
	"path/filepath"
)

// Resolve returns the absolute, symlink-resolved path to the currently running executable. If the
// executable path can't be resolved through symlinks (e.g. it doesn't exist as a real file, only
// relevant in unusual environments), it falls back to the unresolved path from os.Executable
// rather than failing outright — a working, if unresolved, path is more useful to the caller than
// no path at all.
func Resolve() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, nil
	}
	return resolved, nil
}
