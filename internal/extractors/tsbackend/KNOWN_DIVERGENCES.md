# Known divergences from the regex backend

Every deliberate behavioral difference between the tree-sitter backend (`BUILD82_EXTRACTOR_BACKEND=treesitter`)
and the default regex/bracket-depth backend, per `.dev/docs/10-treesitter-backend-plan.md` §3.4: where
tree-sitter is more correct on a known regex blind spot, the difference is documented here rather than
replicated as a bug-for-bug match.

## 1. Commented-out array entries — general pattern, not just `version.php` (Phase 1, confirmed recurring in Phase 5)

**Regex backend**: matches a commented-out line, e.g. `// $plugin->component = 'old_name';`, because
its line-based `strings.Contains` dispatch has no comment-stripping pass — a known, deliberately
preserved imprecision (see `internal/extractors/plugin.go`'s `readVersionPhp` doc comment and
`plugin_test.go`'s `TestDetectPlugin_CommentedOutComponentStillMatches`).

**Tree-sitter backend**: correctly parses the line as a `comment` node, not an `assignment_expression`
— it finds no `$plugin->component` assignment at all, so `DetectPlugin` falls back to
`{type}_{dirname}` exactly like it does for a plugin with no `version.php` component field at all.

**Which is more correct**: tree-sitter. A commented-out line has no runtime effect in real PHP; the
regex backend's behavior here is an accepted historical quirk, not a feature worth preserving in the
new backend.

Verified in `internal/extractors/tsbackend/version_test.go`'s
`TestReadVersionPhp_KnownDivergence_CommentedOutComponent`.

**This is a general pattern, not specific to `version.php`.** Every regex-backend extractor built on
`phparray.ExtractArrayBody`/`SplitIntoBlocks`/`SplitKeyedEntries` operates on raw file text with no
comment-stripping pass, so any of them will match an entry inside a `/* ... */` or `//` comment block
just as readily as a real one. Confirmed recurring in Phase 5 against real core source: several real
`db/access.php` files (e.g. `mod/url`, `mod/folder`, `mod/imscp`, `mod/page`, `mod/resource` in
dev-uvv) have a capability entirely commented out inside `/* ... */` (typically with a `TODO: review
...` note) — the regex backend counts it as a real capability, tree-sitter correctly does not. Expect
this same category to recur in any later phase's real-file regression too; it is not itself a new
finding each time, just a restatement of this same entry.

## 2. Any string field — escaped apostrophe truncation (Phase 4)

**Regex backend**: `phparray.ExtractString`'s capture group is `['"]([^'"]+)['"]` — it stops at the
first quote character it sees, including an *escaped* one. A real value like
`'Update a user\'s preferences'` gets truncated to `Update a user\` (with a dangling backslash) —
the regex has no concept of an escape sequence, so `\'` doesn't protect the quote from ending the
match.

**Tree-sitter backend**: correctly parses `\'` as an `escape_sequence` node inside the `string`,
never mistaking it for the closing quote — returns the full, correct `Update a user's preferences`.

**Which is more correct**: tree-sitter. Found against real Moodle source
(`core_user_update_user_preferences`, `core_competency_list_user_plans`, and others across
`dev-500`/`dev-uvv`'s `lib/db/services.php`) — this is common enough in real descriptions
(Portuguese/English text with contractions) to matter, not a contrived edge case.

## 3. Any string field — multi-line string concatenation (Phase 4)

**Regex backend**: when a value is written as `'first part ' . 'second part'` (Moodle's own
`db/services.php` does this routinely for long descriptions and capability lists), the regex only
ever captures the *first* quoted fragment and silently drops everything after the `.` — it has no
concept of the concatenation operator.

**Tree-sitter backend**: `StringValue` recursively resolves `.`-operator `binary_expression` nodes
(see its own doc comment and `shared_test.go`'s `TestStringValue_Concatenation`/
`TestStringValue_ConcatenationThreeFragments`), correctly returning the full concatenated string.

**Which is more correct**: tree-sitter. Found against real Moodle core source in `dev-uvv`'s and
`dev-500`'s `lib/db/services.php` (e.g. `core_grades_grader_gradingpanel_point_fetch`'s description,
`core_course_update_courses`'s capability list) — common in core's own long-form descriptions.

Both #2 and #3 mean `internal/extractors/services_test.go`'s real-file parity test treats
`Description`/`Capabilities` differences as expected (logged, not failed) rather than asserting
strict equality on those two fields specifically — every other field is still asserted exactly.

## 4. `db/access.php` — double-quoted capability keys, and compound riskbitmask expressions (Phase 5)

**Regex backend**: `capabilityKeyPattern` is deliberately single-quote-only (see its own doc
comment in `internal/extractors/capabilities.go`) — a double-quoted capability key like
`"local/test:x" => [...]` is silently never matched at all (confirmed by the regex backend's own
test, `TestParseAccessPhp_DoubleQuotedKeyNotMatched`). Separately, `extractCapabilityString`'s bare-
constant fallback regex (`[A-Z_0-9]+`) stops at the first non-identifier character, so a compound
bitwise-OR expression like `RISK_SPAM | RISK_PERSONAL | RISK_XSS` (routine in Moodle core's own
`lib/db/access.php`) truncates to just `RISK_SPAM`.

**Tree-sitter backend**: has no quote-style restriction on keys (any string literal is a string
literal), so it finds double-quoted capability keys too. For `contextlevel`/`riskbitmask`/archetype
values, `capabilityStringOrConstant` falls back to the value node's full raw source text rather than
a narrow regex match, so a compound expression comes back complete:
`RISK_SPAM | RISK_PERSONAL | RISK_XSS`, not just the first flag.

**Which is more correct**: tree-sitter, in both cases — the double-quote restriction was never a
real Moodle constraint (just a regex limitation), and truncating a bitmask to one flag out of
several is a real information loss, not a stylistic quirk. Verified against real core source
(`lib/db/access.php`'s multi-flag `riskbitmask` entries) in
`internal/extractors/tsbackend/capabilities_test.go`'s `TestParseAccessPhp_CompoundRiskBitmask` and
`TestParseAccessPhp_KnownDivergence_DoubleQuotedKeyIsMatched`.

## 5. Multi-line `implements` clauses truncated by `(?m)`'s line-anchored `$` (Phase 7)

**Regex backend**: `implementsPattern` is
``(?m)\bimplements\s+([a-zA-Z_\\][a-zA-Z0-9_,\\\s]+?)(?:\{|$)``. The `(?m)` flag makes `$` match at
the end of *every line*, not just the end of the whole decl string. Combined with the lazy `+?`
quantifier (which stops as early as the pattern allows), a multi-line `implements` clause like:

```php
class provider implements
        \core_privacy\local\metadata\provider,
        \core_privacy\local\request\subsystem\provider,
        \core_privacy\local\request\core_userlist_provider {
```

only captures the *first* interface — the lazy match is satisfied by `$` at the end of the first
line, long before it would need to keep scanning to find the real `{`. Every interface after the
first line is silently dropped.

**Tree-sitter backend**: reads the `class_interface_clause` node directly, which structurally
contains every interface listed regardless of how many lines they span.

**Which is more correct**: tree-sitter. Found against real core source
(`lib/classes/privacy/provider.php`, and several `router/parameters/*.php` files, all in
`dev-500`) — multi-line, multi-interface `implements` clauses are a normal, common style in
Moodle core itself, not a contrived edge case.

## 6. `db/renamedclasses.php` — old class names containing a namespace separator (Phase 7)

**Regex backend**: `renamedClassEntryPattern` is `'([a-zA-Z0-9_]+)'\s*=>\s*([^,\]]+)`. The *key*
capture group (`[a-zA-Z0-9_]+`) has no backslash in its character class, so it cannot match an old
class name that itself contains a namespace separator, e.g.
`'core_reportbuilder\\report_access_exception' => '...'`. Real core source
(`lib/db/renamedclasses.php`) renames namespaced classes routinely — of 4 real entries in that
file, the regex backend only matches the one whose old name has no backslash at all, silently
dropping the other 3.

**Tree-sitter backend**: reads the key as whatever string literal it actually is, with no
character-class restriction — correctly finds all 4.

**Which is more correct**: tree-sitter. This isn't a stylistic edge case — the majority of real
renamed-class entries in Moodle core involve a namespaced old name, so the regex backend's
practical miss rate here is high.

## 7. `final class` declarations are invisible to the regex backend entirely (Phase 7)

**Regex backend**: `kindPattern` is `(?m)^(abstract\s+class|class|interface|trait|enum)\s+(...)`  —
anchored at the *start of the line* (`(?m)^`). A line reading `final class hooks implements ... {`
starts with `final`, not one of the four alternatives, so the whole line never matches — the class
isn't just miscategorized, it's **never detected at all**.

**Tree-sitter backend**: `class_declaration` is the node type regardless of any modifier
(`abstract`, `final`, or none) — `final class` is found exactly like a plain `class`, with an
`abstract_modifier` check used only to distinguish "class" from "abstract class" (§`classKind`).

**Which is more correct**: tree-sitter. Confirmed against real core source — `final class hooks`
(`lib/classes/hooks.php`), `final class ip_utils` (`lib/classes/ip_utils.php`), and several other
`final class` declarations under `lib/classes/hook/` are completely invisible to the regex backend.
`final` is an increasingly common modern-PHP convention for utility/value classes not meant to be
subclassed — this isn't a rare pattern.

Entries #5, #6, and #7 mean `internal/extractors/classes_test.go`'s real-file parity tests compare
by FQN (for classes) and log-not-fail on any count mismatch (for both classes and renamed-class
entries) rather than asserting exact positional/count equality — see
`TestExtractClasses_TreesitterBackendParity_RealFiles` and
`TestParseRenamedClassesPhp_TreesitterBackendParity_RealFiles`.

## 8. Reference-returning functions (`function &name()`) invisible to the regex backend (Phase 9)

**Regex backend**: `funcPattern` is `^function\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(` — the character
right after `function\s+` must be a valid identifier start. PHP's reference-return syntax,
`function &get_mimetypes_array() { ... }`, puts `&` there instead — the whole line never matches,
so the function isn't just misclassified, it's **never found at all**.

**Tree-sitter backend**: parses `function &name(...)` as an ordinary `function_definition` with a
`name` child, regardless of the reference-return marker.

**Which is more correct**: tree-sitter. Confirmed against real core source —
`function &get_mimetypes_array()` in `lib/filelib.php` (present in every real installation sampled)
is a genuine, actively-used core function, not a contrived edge case.

## 9. PHP 8 attributes (`#[...]`) between a PHPDoc block and a function break the regex backend's doc lookup (Phase 9)

**Regex backend**: `findDocBlock` walks backward from the function line looking for the docblock's
closing `*/`, tolerating only blank lines in between. Modern Moodle core (5.0+) annotates deprecated
functions with a PHP 8 attribute directly above the function, e.g.:

```php
/**
 * @deprecated since 4.2 Use \core\cron::run_main_process() instead.
 */
#[\core\attribute\deprecated('\core\cron::run_main_process()', since: '4.2', mdl: 'MDL-77186', final: true)]
function cron_run() { ... }
```

The `#[...]` line is neither blank nor `*/`, so the backward walk aborts immediately without ever
finding the real docblock — the function is classified `unverified` even though it has a complete,
`@deprecated`-tagged PHPDoc block right above it.

**Tree-sitter backend**: the PHP grammar attaches a leading attribute directly to the
`function_definition` node it decorates (confirmed by inspection — the attribute text is part of
that same node's span, not a separate sibling), so `PrevSibling()` still correctly finds the
preceding `comment` with no special-casing needed.

**Which is more correct**: tree-sitter. This is the single largest source of real-file
disagreement found in this phase — dozens of functions in `lib/deprecatedlib.php` across every
installation sampled are misclassified `unverified` by the regex backend when they are actually
`deprecated` (or, for non-deprecated attributes on ordinary functions, `public`) — a real,
practically significant miss, not a rare corner case, since Moodle 5.0 adopted this attribute
convention broadly for its own deprecation marking.

## 10. A malformed PHPDoc continuation line (missing the `*` prefix) breaks the regex backend's opening-delimiter search (Phase 9)

**Regex backend**: `findDocBlock` walks backward from the closing `*/` looking for the opening
`/**`, and aborts the search the moment it hits a non-blank line that doesn't start with `*`. Real
core source (`lib/datalib.php`'s `add_to_config_log`) has a wrapped `@param` description whose
continuation line has no `*` prefix at all:

```php
 * @param    string  $name     The name of the configuration change action
                               For example 'filter_active' when activating or deactivating a filter
 * @param    string  $oldvalue The config setting's previous value
```

Walking backward from `*/`, the regex hits this bare continuation line before reaching the real
`/**` opener and gives up — the entire docblock (multiple `@param`/`@return` tags included) is
missed, and the function is classified `unverified`.

**Tree-sitter backend**: a block comment is a single lexer token from `/*` to the matching `*/`
regardless of what's inside it — the malformed continuation line doesn't affect where the comment
node starts or ends, so the whole docblock is found and parsed normally.

**Which is more correct**: tree-sitter. A missing `*` on a wrapped doc line is a common enough
real-world PHPDoc authoring slip that the regex backend's fragility here is a real, not contrived,
gap.

## 11. A non-standard closing marker (`**/` instead of `*/`) breaks the regex backend's closing-delimiter detection (Phase 9)

**Regex backend**: the docblock-closing check is an exact match, `trimmed == "*/"`. Real core source
(`lib/filterlib.php`'s `filter_save_tags`) closes its docblock with `**/` (a doubled asterisk) —
this never equals `"*/"`, so the backward walk never recognizes the docblock as closed and the
whole block is missed.

**Tree-sitter backend**: a block comment simply ends at the first `*/` it encounters while
lexing — `**/`'s last two characters are exactly `*/`, so the comment token ends there correctly
regardless of the extra leading asterisk.

**Which is more correct**: tree-sitter. A doubled closing asterisk is a harmless, common typo that
doesn't change the comment's meaning in real PHP — the regex backend's exact-string match treats it
as if the comment never closed at all.

Entries #8–#11 mean `internal/extractors/api_test.go`'s real-file cross-backend visibility diff
(`TestExtractFunctionsFromPhpFile_TreesitterBackendParity_RealLibFiles` — the primary correctness
check for this extractor, per `10-treesitter-backend-plan.md` §5.3, given no original test suite)
logs every disagreement for visibility rather than failing on it: every single disagreement found
across all 4 real installations' `lib/*.php` files was manually triaged during Phase 9 and traced to
one of these four root causes, all tree-sitter-more-correct.
