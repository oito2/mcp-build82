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
# questions in natural language that exercise the tools' structured output. Each answer must
# mention an expected string. The build82 configuration comes from BUILD82_MOODLE_PATH in the MCP
# entry, so ~/.build82 is neither read nor written.
#
# Usage, from the module root:
#   scripts/smoke/claude.sh [moodle-root]
# Without an argument, a copy of testdata/moodle is used. Needs the `claude` CLI, logged in.
# Exits with status 1 when any answer lacks its expected string.

set -eu

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [ "$#" -ge 1 ]; then
  moodle="$1"
else
  cp -R testdata/moodle "$work/moodle"
  moodle="$work/moodle"
fi

go build -o "$work/build82" ./cmd/build82

cat > "$work/mcp.json" <<JSON
{"mcpServers": {"build82": {"command": "$work/build82", "args": [], "env": {"BUILD82_MOODLE_PATH": "$moodle"}}}}
JSON

failed=0

# ask runs one prompt through Claude Code with only build82's tools allowed and checks that the
# answer contains the expected string (case-insensitive).
ask() {
  prompt="$1"
  expected="$2"
  printf '\n=== %s\n' "$prompt"
  answer="$(cd "$work" && claude -p --strict-mcp-config --mcp-config "$work/mcp.json" \
    --allowedTools "mcp__build82" --output-format text "$prompt" 2>&1)" || true
  printf '%s\n' "$answer"
  if printf '%s' "$answer" | grep -qi -- "$expected"; then
    printf -- '--- ok (found "%s")\n' "$expected"
  else
    printf -- '--- FAILED (expected "%s")\n' "$expected"
    failed=1
  fi
}

ask "Use the build82 tools to regenerate the global Moodle indexes and tell me which Moodle version was detected." "4."
ask "Use build82 to search the installed plugins for 'demo' and give me the component name you found." "local_demo"
ask "Using build82, what is the version number of the plugin local_demo?" "2024010100"
ask "Using build82, generate the context for the plugin local_demo, then list the plugins currently under development." "local_demo"
ask "Run the build82 doctor and tell me its overall verdict in one word." "ok\|warn\|fail"

exit "$failed"
