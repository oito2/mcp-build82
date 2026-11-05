#!/bin/sh
# Copyright (C) 2026  OITO2
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

# Real-client smoke test: builds build82, registers it for one Claude Code session only
# (--strict-mcp-config, so no other configured MCP server is loaded), and asks Claude Code a few
# questions in natural language that exercise the tools' structured output, plus the
# upgrade_plugin prompt. Questions whose answer could be a common word or number ask for a fixed
# "key=value" form, and each answer must match an extended regular expression (case-insensitive);
# a failed `claude` call fails its question. The build82 configuration
# comes from BUILD82_MOODLE_PATH in the MCP entry, so ~/.build82 is neither read nor written. The
# coding-style question runs only when phpcs with the moodle standard is on PATH, and the version
# bump only on the copy of testdata/moodle, never on a Moodle root given as argument.
#
# Usage, from the module root:
#   scripts/smoke/claude.sh [moodle-root]
# Without an argument, a copy of testdata/moodle is used. Needs the `claude` CLI, logged in, and
# jq. Exits with status 1 when any question fails.

set -eu

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The fixture is Moodle 4.3; any MAJOR.MINOR is accepted for a Moodle root given as argument.
if [ "$#" -ge 1 ]; then
  moodle="$1"
  moodle_version='[0-9]+\.[0-9]+'
else
  cp -R testdata/moodle "$work/moodle"
  moodle="$work/moodle"
  moodle_version='4\.3([^0-9]|$)'
fi

go build -o "$work/build82" ./cmd/build82

# jq escapes the paths, so quotes or backslashes in them cannot break the JSON.
jq -n --arg cmd "$work/build82" --arg moodle "$moodle" \
  '{mcpServers: {build82: {command: $cmd, args: [], env: {BUILD82_MOODLE_PATH: $moodle}}}}' \
  > "$work/mcp.json"

failed=0

# ask runs one prompt through Claude Code with only build82's tools allowed and checks that the
# call succeeds and that the answer matches the extended regular expression `pattern`
# (case-insensitive).
ask() {
  prompt="$1"
  pattern="$2"
  printf '\n=== %s\n' "$prompt"
  status=0
  answer="$(cd "$work" && claude -p --strict-mcp-config --mcp-config "$work/mcp.json" \
    --allowedTools "mcp__build82" --output-format text "$prompt" </dev/null 2>&1)" || status=$?
  printf '%s\n' "$answer"
  if [ "$status" -ne 0 ]; then
    printf -- '--- FAILED (claude exited with status %s)\n' "$status"
    failed=1
  elif printf '%s' "$answer" | grep -qiE -- "$pattern"; then
    printf -- '--- ok (matched "%s")\n' "$pattern"
  else
    printf -- '--- FAILED (expected a match for "%s")\n' "$pattern"
    failed=1
  fi
}

ask "Use the build82 tools to regenerate the global Moodle indexes and tell me which Moodle version was detected. End your answer with a line of the form moodle=<major.minor>." \
  "moodle=$moodle_version"
ask "Use build82 to search the installed plugins for 'demo' and give me the component name you found." "local_demo"
ask "Using build82, what is the version number of the plugin local_demo?" "2024010100"
ask "Using build82, generate the context for the plugin local_demo, then list the plugins currently under development." "local_demo"
ask "Run the build82 doctor and tell me its overall verdict. End your answer with a line of the form verdict=<ok|warn|fail>." \
  "verdict=(ok|warn|fail)([^a-z]|\$)"
ask "Run the build82 doctor and tell me the status of its Upgrade Consistency and Privacy API sections. End your answer with a line of the form upgrade=<status> privacy=<status>, using the doctor's status words." \
  "upgrade=ok[^a-z].*privacy=ok([^a-z]|\$)"
ask "/mcp__build82__upgrade_plugin local_demo 4.5" "before_footer_html_generation"

if phpcs -i 2>/dev/null | sed 's/.* are //; s/ and /, /g' | tr ',' '\n' | sed 's/^ *//; s/ *$//' | grep -qx moodle; then
  ask "Using build82, check local_demo against the Moodle coding style and tell me how many PHP files were checked. End your answer with a line of the form files=<number>." \
    "files=12([^0-9]|\$)"
else
  printf '\n=== coding style: skipped (phpcs with the moodle standard is not on PATH)\n'
fi

if [ "$#" -eq 0 ]; then
  ask "Using build82, bump the version of local_demo to 2026100900 without an upgrade step, and tell me the old and new version numbers." "2026100900"
fi

exit "$failed"
