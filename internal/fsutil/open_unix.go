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

//go:build unix

package fsutil

import "syscall"

// openFlags are added to every OpenRegular: O_NONBLOCK keeps opening a FIFO with no writer from
// waiting forever (it has no effect on a regular file), and O_NOFOLLOW makes opening a symbolic
// link at the last path element fail instead of following it.
const openFlags = syscall.O_NONBLOCK | syscall.O_NOFOLLOW

// refuseSymlinkFirst reports whether OpenRegular must reject a symbolic link with an Lstat before
// opening. It is false here, where openFlags already makes the open itself refuse one.
const refuseSymlinkFirst = false
