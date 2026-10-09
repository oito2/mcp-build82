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

package prompt

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// TestConfirm verifies the answers counted as yes, that a closed input is a "no", and that the
// question is printed.
func TestConfirm(t *testing.T) {
	for input, want := range map[string]bool{"y\n": true, " Y \n": true, "y": true, "yes\n": false, "n\n": false, "\n": false, "": false} {
		var out strings.Builder
		got, err := Confirm(context.Background(), bufio.NewReader(strings.NewReader(input)), &out, "Go? ")
		if err != nil || got != want {
			t.Errorf("Confirm(%q) = %v, %v; want %v", input, got, err, want)
		}
		if !strings.HasPrefix(out.String(), "Go? ") {
			t.Errorf("printed %q, want the question", out.String())
		}
	}
}

// TestReadLine_ReportsEOF verifies that a line ending at the end of input is returned with eof set.
func TestReadLine_ReportsEOF(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("first\nlast"))
	if line, eof, err := ReadLine(context.Background(), in); line != "first\n" || eof || err != nil {
		t.Errorf("first line = %q, eof=%v, err=%v", line, eof, err)
	}
	if line, eof, err := ReadLine(context.Background(), in); line != "last" || !eof || err != nil {
		t.Errorf("last line = %q, eof=%v, err=%v", line, eof, err)
	}
}

// TestConfirm_StopsWhenTheContextEnds verifies that a question waiting on an input that never
// answers returns ErrInterrupted (wrapping context.Canceled) as soon as the context is cancelled,
// as a Ctrl-C does.
func TestConfirm_StopsWhenTheContextEnds(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Confirm(ctx, bufio.NewReader(r), io.Discard, "Go? ")
	if !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want ErrInterrupted wrapping context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Confirm returned %s after the cancellation", elapsed)
	}
}

// TestReadLine_AlreadyCancelled verifies that an ended context is reported before any read.
func TestReadLine_AlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ReadLine(ctx, bufio.NewReader(strings.NewReader("y\n"))); !errors.Is(err, ErrInterrupted) {
		t.Errorf("err = %v, want ErrInterrupted", err)
	}
}
