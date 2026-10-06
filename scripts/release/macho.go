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

package main

import (
	"debug/macho"
	binenc "encoding/binary"
	"fmt"
	"os"
)

// Constants of the universal ("fat") Mach-O file format: header magic and record sizes in bytes,
// the 64-bit CPU type flag, and the log2 of the slice alignment.
const (
	fatMagic       = 0xcafebabe
	fatHeaderSize  = 8
	fatArchSize    = 20
	cpuArchABI64   = 0x01000000
	pageAlignShift = 14 // 16 KiB: the arm64 page size, also a valid (larger) alignment for x86_64
)

// writeUniversalMachO combines the thin Mach-O executables at the `thin` paths (one per CPU
// architecture) into a single universal ("fat") binary at `out`, with mode 0755. The layout matches
// `lipo -create`: a big-endian fat_header, one fat_arch record per slice, then each slice copied
// unmodified at a 16 KiB-aligned offset, so any code signature embedded in a slice stays valid.
// It returns an error when fewer than two slices are given, when an input is not a thin 64-bit
// Mach-O file, when two inputs share a CPU type, or when reading or writing fails.
func writeUniversalMachO(out string, thin ...string) error {
	if len(thin) < 2 {
		return fmt.Errorf("universal binary needs at least 2 slices, got %d", len(thin))
	}

	type slice struct {
		data   []byte
		cpu    macho.Cpu
		subCpu uint32
	}
	slices := make([]slice, 0, len(thin))
	seen := make(map[macho.Cpu]string, len(thin))
	for _, path := range thin {
		f, err := macho.Open(path)
		if err != nil {
			return fmt.Errorf("%s is not a thin Mach-O file: %w", path, err)
		}
		cpu, subCpu := f.Cpu, f.SubCpu
		f.Close()
		if cpu&cpuArchABI64 == 0 {
			return fmt.Errorf("%s: only 64-bit slices are supported, got %v", path, cpu)
		}
		if prev, dup := seen[cpu]; dup {
			return fmt.Errorf("%s and %s have the same CPU type %v", prev, path, cpu)
		}
		seen[cpu] = path

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		slices = append(slices, slice{data: data, cpu: cpu, subCpu: subCpu})
	}

	const align = 1 << pageAlignShift
	alignUp := func(n int) int { return (n + align - 1) &^ (align - 1) }

	header := make([]byte, fatHeaderSize+fatArchSize*len(slices))
	binenc.BigEndian.PutUint32(header[0:], fatMagic)
	binenc.BigEndian.PutUint32(header[4:], uint32(len(slices)))

	offsets := make([]int, len(slices))
	next := alignUp(len(header))
	for i, s := range slices {
		offsets[i] = next
		rec := header[fatHeaderSize+fatArchSize*i:]
		binenc.BigEndian.PutUint32(rec[0:], uint32(s.cpu))
		binenc.BigEndian.PutUint32(rec[4:], s.subCpu)
		binenc.BigEndian.PutUint32(rec[8:], uint32(next))
		binenc.BigEndian.PutUint32(rec[12:], uint32(len(s.data)))
		binenc.BigEndian.PutUint32(rec[16:], pageAlignShift)
		next = alignUp(next + len(s.data))
	}

	buf := make([]byte, offsets[len(offsets)-1]+len(slices[len(slices)-1].data))
	copy(buf, header)
	for i, s := range slices {
		copy(buf[offsets[i]:], s.data)
	}
	return os.WriteFile(out, buf, 0o755)
}
