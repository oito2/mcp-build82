# Backend benchmark

Phase 10.3 (`.dev/docs/11-treesitter-implementation-phases.md`) — the regex-vs-tree-sitter latency
cost the roadmap flagged as real, measured rather than assumed. Benchmark lives in
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
