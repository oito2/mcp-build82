# Backend benchmark

The regex-vs-tree-sitter latency cost, measured rather than assumed. The benchmark lives in
`internal/extractors/api_bench_test.go` (`BenchmarkExtractFunctionsFromPhpFile_Regex` /
`_Treesitter`), run against the largest real file available in this environment:
`/srv/workspace/www/html/mdle/dev-500/lib/moodlelib.php` (~10,136 lines). Skips gracefully if that
path isn't present.

## Results (this machine, 13th Gen Intel Core i5-13500, `go test -bench . -benchtime=10x -benchmem`)

| Backend | ns/op | B/op | allocs/op |
|---|---|---|---|
| Regex (default) | 1,469,481 (~1.47 ms) | 1,391,109 (~1.4 MB) | 6,751 |
| Tree-sitter (`BUILD82_EXTRACTOR_BACKEND=treesitter`) | 376,730,773 (~377 ms) | 163,251,042 (~163 MB) | 23,368 |

**Tree-sitter is ~256x slower and allocates ~117x more memory per call than the regex backend on
this file.** In absolute terms, ~377 ms for a single ~10k-line file is still well within
interactive latency for the tool's actual per-call use (`generate_plugin_context` on one plugin,
`explain_plugin`, etc.) — the cost only compounds for a *full-tree* scan
(`ExtractMoodleApi`/`update_indexes --include_plugins` walking every `lib.php`-shaped file across a
large installation), where hundreds of such files could turn a sub-second regex scan into a
multi-second-to-low-minutes tree-sitter one.

This is exactly why the backend stays opt-in (`BUILD82_EXTRACTOR_BACKEND=treesitter`), never the
default (`10-treesitter-backend-plan.md` §3.3/§7) — a real, not hypothetical, latency/memory cost
in exchange for the correctness improvements in `KNOWN_DIVERGENCES.md`. Users who want maximum
correctness (e.g. auditing `deprecatedlib.php` for PHP 8 attribute-annotated functions, entry #9)
can opt in per-run; everyone else keeps the regex backend's speed by default.

## Large file: 1.1 MB `lib.php`

`BenchmarkLargeLibPhp` (`largefile_test.go`) parses a synthetic `lib.php` of about 1.1 MB with
21,001 top-level functions and runs both root-level scans on it: `ExtractFunctionsFromPhpFile` and
`DetectLegacyCallbacks` (each parses the file once). Same machine,
`GOMAXPROCS=4 go test -bench BenchmarkLargeLibPhp -benchtime=5x -benchmem`:

| Version | Time for both scans | B/op | allocs/op |
|---|---|---|---|
| Child loops with `NamedChildCount`/`NamedChild(i)`, one root scan per legacy callback name | ~34.9 s (measured by `TestLargeLibPhp_ScansInLinearTime`) | — | — |
| Single-pass `namedChildren`, top-level function names collected once | 705,927,171 ns (~0.71 s, ~0.35 s per parse) | 320,643,620 (~321 MB) | 45,677 |

In gotreesitter `NamedChild(i)` and `NamedChildCount()` each walk all children, so a loop over
them is quadratic in the number of children. `ParseFile` also refuses files over 2 MiB and parses
with a 10-second timeout; a parse that stops early is an error, not a partial tree.
