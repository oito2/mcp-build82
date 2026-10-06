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

// Package binpath resolves the path to the currently running build82 binary. Shared by
// internal/installer and internal/selfupdate, which each need this to locate/replace the running
// executable, using os.Executable followed by filepath.EvalSymlinks.
package binpath

import (
	"fmt"
	"os"
	"path/filepath"
)

// Resolve returns the absolute, symlink-resolved path to the currently running executable. If
// symlink resolution fails, it returns the unresolved path from os.Executable with a nil error.
// The error is non-nil only when os.Executable itself fails. It never panics.
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
