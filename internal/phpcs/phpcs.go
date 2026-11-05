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

// Package phpcs detects PHP_CodeSniffer (phpcs) with the Moodle coding standard and runs it on
// plugin files, parsing its JSON report.
package phpcs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Standard is the name of the Moodle coding standard, installed by moodlehq/moodle-cs.
const Standard = "moodle"

// InstallHint tells how to install phpcs with the Moodle coding standard.
const InstallHint = "install it with `composer global require moodlehq/moodle-cs` (which brings phpcs and the moodle standard) and make sure composer's global bin directory is on PATH"

// Timeouts of the phpcs invocations. They are variables so tests can shorten them.
var (
	// InfoTimeout bounds `phpcs -i`.
	InfoTimeout = 30 * time.Second
	// RunTimeout bounds a coding-style check.
	RunTimeout = 120 * time.Second
	// waitDelay is how long Wait waits for the output pipes after the process was killed on
	// timeout, so a child that keeps them open (a wrapper script's subprocess) cannot hang it.
	waitDelay = 5 * time.Second
)

// State is the availability of phpcs with the Moodle standard.
type State int

// Possible States.
const (
	// Missing means phpcs is not on PATH.
	Missing State = iota
	// Unusable means phpcs is on PATH but `phpcs -i` failed.
	Unusable
	// NoStandard means phpcs works but the moodle standard is not installed.
	NoStandard
	// Ready means phpcs works and the moodle standard is installed.
	Ready
)

// Status is the result of Check: the State, and for Unusable the reason.
type Status struct {
	State  State
	Detail string
}

// Check looks phpcs up on PATH and runs `phpcs -i` (bounded by InfoTimeout) to see whether the
// moodle standard is installed.
func Check(ctx context.Context) Status {
	path, err := exec.LookPath("phpcs")
	if err != nil {
		return Status{State: Missing}
	}
	ctx, cancel := context.WithTimeout(ctx, InfoTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-i")
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Status{State: Unusable, Detail: fmt.Sprintf("`phpcs -i` did not finish within %s", InfoTimeout)}
		}
		return Status{State: Unusable, Detail: fmt.Sprintf("`phpcs -i` failed: %s", firstLine(stderr.String(), stdout.String(), err.Error()))}
	}
	for _, s := range InstalledStandards(stdout.String()) {
		if s == Standard {
			return Status{State: Ready}
		}
	}
	return Status{State: NoStandard}
}

// InstalledStandards parses the output of `phpcs -i` ("The installed coding standards are A, B
// and C") into the standard names.
func InstalledStandards(out string) []string {
	out = strings.TrimSpace(out)
	if i := strings.Index(out, " are "); i >= 0 {
		out = out[i+len(" are "):]
	}
	out = strings.TrimSuffix(out, ".")
	var names []string
	for _, part := range strings.Split(strings.ReplaceAll(out, " and ", ", "), ",") {
		if name := strings.TrimSpace(part); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Message is one violation of a phpcs JSON report.
type Message struct {
	Message  string `json:"message"`
	Source   string `json:"source"`
	Severity int    `json:"severity"`
	Fixable  bool   `json:"fixable"`
	Type     string `json:"type"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

// FileReport is the part of a phpcs JSON report about one file.
type FileReport struct {
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Messages []Message `json:"messages"`
}

// Totals are the counts of a phpcs JSON report.
type Totals struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Fixable  int `json:"fixable"`
}

// Report is a phpcs JSON report: the totals and each checked file, keyed by the path phpcs
// reported.
type Report struct {
	Totals *Totals                `json:"totals"`
	Files  map[string]*FileReport `json:"files"`
}

// ErrTimeout reports a check that did not finish within RunTimeout.
var ErrTimeout = errors.New("phpcs did not finish in time")

// Run checks `paths` (files or directories) against the moodle standard, running phpcs in `dir`
// bounded by RunTimeout: PHP files only, skipping .build82, vendor and node_modules directories,
// and warnings only when `includeWarnings` is true. Every path must be absolute: phpcs does not
// honour "--", so a relative path starting with "-" would be read as an option (such as
// --bootstrap or --report-file); an absolute path starts with a separator or a volume name and
// never can. phpcs exits with a non-zero code when it finds violations, so the result is taken
// from the JSON report whatever the exit code. It returns an error for a relative path, an error
// wrapping ErrTimeout on timeout, and an error with phpcs's own message when no report is produced.
func Run(ctx context.Context, dir string, paths []string, includeWarnings bool) (*Report, error) {
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("phpcs.Run: path %q must be absolute", p)
		}
	}
	path, err := exec.LookPath("phpcs")
	if err != nil {
		return nil, err
	}
	args := []string{
		"--standard=" + Standard, "--report=json", "--extensions=php", "-q",
		"--ignore=*/.build82/*,*/vendor/*,*/node_modules/*",
	}
	if !includeWarnings {
		args = append(args, "--warning-severity=0")
	}
	args = append(args, paths...)

	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%w (limit %s)", ErrTimeout, RunTimeout)
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.Totals == nil {
		reason := "no JSON report"
		if runErr != nil {
			reason = runErr.Error()
		}
		return nil, fmt.Errorf("phpcs produced no report: %s", firstLine(stderr.String(), stdout.String(), reason))
	}
	return &report, nil
}

// firstLine returns the first non-blank line of the first of `texts` that has one.
func firstLine(texts ...string) string {
	for _, t := range texts {
		for _, line := range strings.Split(t, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return line
			}
		}
	}
	return ""
}
