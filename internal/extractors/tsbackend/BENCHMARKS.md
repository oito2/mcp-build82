# Backend benchmark

The regex-vs-tree-sitter latency cost, measured rather than assumed. The benchmark lives in
`internal/extractors/api_bench_test.go` (`BenchmarkExtractFunctionsFromPhpFile_Regex` /
`_Treesitter`), run against the largest real core file available in this environment:
`lib/moodlelib.php` of the `mdle-500` installation under `BUILD82_REAL_MOODLE_DIR` (about 10k
lines). It is skipped when that file is not present.

## Results (13th Gen Intel Core i5-13500, `GOMAXPROCS=4 go test -bench . -benchtime=10x -benchmem`)

| gotreesitter | Backend | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| v0.55.1 | Regex (default) | 1,619,642 (~1.6 ms) | 1,918,385 (~1.9 MB) | 5,761 |
| v0.55.1 | Tree-sitter | 76,676,385 (~77 ms) | 2,378,805 (~2.4 MB) | 7,834 |
| v0.56.0 | Regex (default) | 1,598,777 (~1.6 ms) | 1,910,976 (~1.9 MB) | 5,760 |
| v0.56.0 | Tree-sitter | 71,688,285 (~72 ms) | 2,144,282 (~2.1 MB) | 6,742 |

**Tree-sitter is ~45x slower than the regex backend on this file** (the earlier gap of ~250x shrank
with the single-pass child iteration and later gotreesitter releases). About 72 ms for a
10k-line file is well within interactive latency for per-plugin calls (`generate_plugin_context`,
`explain_plugin`); the cost only compounds on a full-tree scan (`update_indexes` with
`include_plugins` over a large installation), where hundreds of files turn a sub-second regex scan
into seconds.

This is why the backend stays opt-in (`BUILD82_EXTRACTOR_BACKEND=treesitter`), never the default:
a real latency cost in exchange for the correctness improvements in `KNOWN_DIVERGENCES.md`.
Users who want maximum correctness (e.g. auditing `deprecatedlib.php` for PHP 8
attribute-annotated functions, entry #9) can opt in per run.

Upgrading gotreesitter from v0.55.1 to v0.56.0 changed none of the backend differences logged by
the real-file parity tests (the same 80 entries before and after).

## Large file: 1.1 MB `lib.php`

`BenchmarkLargeLibPhp` (`largefile_test.go`) parses a synthetic `lib.php` of about 1.1 MB with
21,001 top-level functions and runs both root-level scans on it: `ExtractFunctionsFromPhpFile` and
`DetectLegacyCallbacks` (each parses the file once). Same machine,
`GOMAXPROCS=4 go test -bench BenchmarkLargeLibPhp -benchtime=5x -benchmem`:

| Version | Time for both scans | B/op | allocs/op |
|---|---|---|---|
| Child loops with `NamedChildCount`/`NamedChild(i)`, one root scan per legacy callback name | ~34.9 s (measured by `TestLargeLibPhp_ScansInLinearTime`) | — | — |
| Single-pass `namedChildren`, top-level function names collected once (gotreesitter v0.55.1) | 704,317,359 ns (~0.70 s, ~0.35 s per parse) | 320,643,598 (~321 MB) | 45,677 |
| Same, gotreesitter v0.56.0 | 747,401,807 ns (~0.75 s, ~0.37 s per parse) | 320,119,945 (~320 MB) | 42,440 |

In gotreesitter `NamedChild(i)` and `NamedChildCount()` each walk all children, so a loop over
them is quadratic in the number of children. `ParseFile` also refuses files over 2 MiB and parses
with a 10-second timeout; a parse that stops early is an error, not a partial tree.
