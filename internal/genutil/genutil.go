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

package genutil

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/fsutil"
)

// ContextDir is the subdirectory every build82-generated file lives under, relative to a
// Moodle or plugin root.
const ContextDir = ".build82"

// GlobalOutputPath returns the path of the generated file `filename` inside `moodlePath`'s ContextDir.
func GlobalOutputPath(moodlePath, filename string) string {
	return filepath.Join(moodlePath, ContextDir, filename)
}

// PluginOutputPath returns the path of the generated file `filename` inside `pluginPath`'s ContextDir.
func PluginOutputPath(pluginPath, filename string) string {
	return filepath.Join(pluginPath, ContextDir, filename)
}

// RelativeOrOriginal returns `absPath` relative to `root` (slash-separated), or `absPath`
// unchanged if it cannot be made relative (e.g. different volumes on Windows). It keeps absolute
// host paths, which can expose directory structure and usernames, out of generated Markdown files
// and resource error messages.
func RelativeOrOriginal(root, absPath string) string {
	if rel, err := filepath.Rel(root, absPath); err == nil {
		return filepath.ToSlash(rel)
	}
	return absPath
}

// GeneratorResult is the outcome of a single generator. Generators report failures through it
// instead of panicking or returning an error, so orchestrators can run them in parallel and still
// produce a complete report.
type GeneratorResult struct {
	File    string
	Success bool
	Error   string // empty when Success
	Skipped bool   // true when the mtime cache determined regeneration was unnecessary
}

// Safely runs `fn` and returns its result. If `fn` panics or returns an error, the panic or error
// is converted into a failed GeneratorResult for `outputFile`, so one broken generator cannot abort
// a batch.
func Safely(outputFile string, fn func() (GeneratorResult, error)) GeneratorResult {
	result, err := func() (r GeneratorResult, e error) {
		defer func() {
			if p := recover(); p != nil {
				e = fmt.Errorf("%v", p)
			}
		}()
		return fn()
	}()
	if err != nil {
		return GeneratorResult{File: outputFile, Success: false, Error: err.Error()}
	}
	return result
}

// Write writes `content` to `filePath` (mode 0644) through a temp file renamed into place
// (fsutil.WriteReplace), creating parent directories as needed. Readers never see a partial file,
// and a symbolic link at `filePath` is replaced instead of followed. It refuses to write when the
// file's directory is a symbolic link (fsutil.RefuseSymlinkedDir), so a link planted as a plugin's
// ContextDir cannot redirect the output outside the tree. Errors are reported in the returned
// result's Error field. It does not touch the mtime cache: RunCached marks the output after a
// successful generation.
func Write(filePath, content string) GeneratorResult {
	if err := WriteFile(filePath, []byte(content)); err != nil {
		return GeneratorResult{File: filePath, Success: false, Error: err.Error()}
	}
	return GeneratorResult{File: filePath, Success: true}
}

// WriteFile writes `data` to `filePath` (mode 0644) like Write. It is also meant for build82's own
// bookkeeping files under ContextDir, such as the .indevelopment marker.
func WriteFile(filePath string, data []byte) error {
	if err := fsutil.RefuseSymlinkedDir(filepath.Dir(filePath)); err != nil {
		return err
	}
	return fsutil.WriteReplace(filePath, data, 0o644)
}

// EscapeMdCell returns `s` safe for interpolation into a Markdown table cell or heading: `|` is
// backslash-escaped and line breaks are replaced with spaces. This stops values taken from plugin
// PHP from breaking a table's column count or opening a new Markdown section.
func EscapeMdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return s
}

// SafeText returns `s` with control characters (including line breaks) and backticks removed, so a
// file or directory name taken from a plugin cannot end a line, close a code span or fence, or
// start a Markdown section. It is meant for text inside a code block, such as a directory tree.
func SafeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '`' {
			return -1
		}
		return r
	}, s)
}

// SafeInline returns `s` safe for a Markdown table cell, heading or list item: SafeText, with `|`
// backslash-escaped as in EscapeMdCell.
func SafeInline(s string) string {
	return strings.ReplaceAll(SafeText(s), "|", "\\|")
}

// Header renders the title/description/timestamp header block that starts every generated
// Markdown file, using `title` as the H1 and `description` as the blockquote.
func Header(title, description string) string {
	return strings.Join([]string{
		"# " + title, "",
		"> " + description, "",
		"_Generated by build82 on " + Timestamp() + "_", "",
		"---", "",
	}, "\n")
}

// Timestamp returns the current time in UTC formatted as "2006-01-02 15:04:05", so generated
// files do not differ by the machine's timezone.
func Timestamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

// RunCached returns a job function that checks the mtime cache for `outputFile` against
// `sources`. If the output is not stale it appends a Skipped success result to `results`;
// otherwise it runs `fn` and appends fn's result. When `fn` succeeds without skipping, the output
// is marked in the cache with `readAt` and `sources` (cache.MtimeCache.MarkAt), so `readAt` must be
// taken before the sources are listed and read: a source saved after it makes the output stale
// again. Appends are guarded by `mu`, so the returned jobs can run concurrently against the same
// `results`/`mu` pair.
func RunCached(results *[]GeneratorResult, mu *sync.Mutex, outputFile string, sources []string, readAt time.Time, fn func() GeneratorResult) func() {
	return func() {
		RunDecided(results, mu, outputFile, sources, readAt, cache.Global.IsStale(outputFile, sources), fn)
	}
}

// RunDecided does RunCached's work for a staleness decision `stale` the caller already took with
// cache.MtimeCache.IsStale (which must not be asked twice for the same output, since it drops the
// mark of a stale output): when `stale` is false it appends a Skipped success result, otherwise it
// runs `fn`, marks the output on success and appends fn's result.
func RunDecided(results *[]GeneratorResult, mu *sync.Mutex, outputFile string, sources []string, readAt time.Time, stale bool, fn func() GeneratorResult) {
	if !stale {
		mu.Lock()
		*results = append(*results, GeneratorResult{File: outputFile, Success: true, Skipped: true})
		mu.Unlock()
		return
	}
	r := fn()
	if r.Success && !r.Skipped {
		cache.Global.MarkAt(outputFile, readAt, sources)
	}
	mu.Lock()
	*results = append(*results, r)
	mu.Unlock()
}

// WriteTableOrPlaceholder writes `header` followed by every entry of `rows` verbatim to `b`
// (each row must carry its own trailing newline). When `rows` is empty it writes a single
// "_(placeholder)_" line instead of a header-only table. `header` may be empty (e.g.
// GeneratePluginDbTables, whose rows each render a full table), in which case only the rows or
// the placeholder are written.
func WriteTableOrPlaceholder(b *strings.Builder, header string, rows []string, placeholder string) {
	if len(rows) == 0 {
		fmt.Fprintf(b, "_(%s)_\n", placeholder)
		return
	}
	if header != "" {
		b.WriteString(header)
	}
	for _, row := range rows {
		b.WriteString(row)
	}
}
