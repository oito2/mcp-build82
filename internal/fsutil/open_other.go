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

//go:build !unix

package fsutil

// openFlags is empty outside unix, where no FIFO can be swapped in for a file.
const openFlags = 0

// refuseSymlinkFirst makes OpenRegular reject a symbolic link with an Lstat before opening, since
// the open itself follows one here.
const refuseSymlinkFirst = true
