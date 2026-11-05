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

// Command fakephpcs stands in for phpcs in tests. Its behavior is set through environment
// variables:
//   - FAKE_PHPCS_STANDARDS: the standards listed by `-i` (default "MySource, PEAR, PSR12 and moodle");
//   - FAKE_PHPCS_MODE: "fail" makes every call exit 3 with an error message, "hang" makes every
//     call sleep for FAKE_PHPCS_HANG (a Go duration, default one hour) and exit, "garbage" makes a
//     check print a non-JSON error and exit 3;
//   - FAKE_PHPCS_REPORT: file printed by a check (exit code 2), after replacing {{ROOT}} with
//     FAKE_PHPCS_ROOT (JSON-escaped);
//   - FAKE_PHPCS_ARGS: file where a check records its working directory and arguments as JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	switch os.Getenv("FAKE_PHPCS_MODE") {
	case "fail":
		fmt.Fprintln(os.Stderr, "PHP Fatal error: phpcs is broken")
		os.Exit(3)
	case "hang":
		d, err := time.ParseDuration(os.Getenv("FAKE_PHPCS_HANG"))
		if err != nil {
			d = time.Hour
		}
		time.Sleep(d)
		os.Exit(0)
	}

	args := os.Args[1:]
	if len(args) == 1 && args[0] == "-i" {
		standards := os.Getenv("FAKE_PHPCS_STANDARDS")
		if standards == "" {
			standards = "MySource, PEAR, PSR12 and moodle"
		}
		fmt.Printf("The installed coding standards are %s\n", standards)
		return
	}

	if path := os.Getenv("FAKE_PHPCS_ARGS"); path != "" {
		wd, _ := os.Getwd()
		data, _ := json.Marshal(map[string]any{"dir": wd, "args": args})
		_ = os.WriteFile(path, data, 0o644)
	}
	if os.Getenv("FAKE_PHPCS_MODE") == "garbage" {
		fmt.Println(`ERROR: the "moodle" coding standard is not installed.`)
		os.Exit(3)
	}
	report, err := os.ReadFile(os.Getenv("FAKE_PHPCS_REPORT"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	root, _ := json.Marshal(os.Getenv("FAKE_PHPCS_ROOT"))
	fmt.Print(strings.ReplaceAll(string(report), "{{ROOT}}", strings.Trim(string(root), `"`)))
	os.Exit(2)
}
