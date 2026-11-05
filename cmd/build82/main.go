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

// Command build82 is an MCP server that helps develop Moodle plugins.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/installer"
	"github.com/oito2/mcp-build82/internal/prompt"
	"github.com/oito2/mcp-build82/internal/selfupdate"
	"github.com/oito2/mcp-build82/internal/server"
	"github.com/oito2/mcp-build82/internal/version"
)

const helpText = `build82 — MCP server for Moodle plugin development

Usage:
  build82                  Run the MCP server over stdio
  build82 install [target] Configure build82 as an MCP server in an AI tool
  build82 self-update      Update to the latest released version
  build82 uninstall [target] Remove build82's MCP server registration

self-update flags:
  --check                Report whether an update is available, without installing it; exits
                          with status 10 when one is available
  --channel <name>        Release channel (default "stable"; reserved for future use)
  --yes, -y               Skip the confirmation prompt before replacing the running binary
  --require-signature     Refuse to update unless cosign v3+ is installed to verify the release
                          signature (by default, without cosign only the checksum is verified)
  --rollback              Swap the binary with its .bak backup (run again to undo) and exit;
                          cannot be combined with other self-update flags

uninstall flags:
  --purge                 Also delete generated files and the config file (asks for its own confirmation)

Flags:
  -h, --help             Show this help text and exit
  --version              Print the version and exit

Run 'build82 <command> --help' for the options of install, uninstall and self-update.
`

// installHelp is the help text of `build82 install --help`.
const installHelp = `Usage: build82 install [target]

Registers build82 as an MCP server in the given AI tool or, with no target, in every detected tool
after confirmation. Asks for the Moodle root first. Close the tool while it runs.

Targets: claude, claude-desktop, antigravity, codex, opencode, cursor, zed, cline

Arguments after "--" are taken as the target even when they start with "-".
`

// uninstallHelp is the help text of `build82 uninstall --help`.
const uninstallHelp = `Usage: build82 uninstall [target] [--purge]

Removes build82's MCP server registration from the given AI tool or, with no target, from every
tool that has one, after confirmation.

  --purge               Also delete generated files and the ~/.build82 config file (asks for its
                        own confirmation)

Targets: claude, claude-desktop, antigravity, codex, opencode, cursor, zed, cline

Arguments after "--" are taken as the target even when they start with "-".
`

// selfUpdateHelp is the help text of `build82 self-update --help`.
const selfUpdateHelp = `Usage: build82 self-update [--check] [--channel <name>] [--yes] [--require-signature]
       build82 self-update --rollback

Updates build82 to the latest release, after confirmation. When cosign v3 or later is on PATH, the
release signature is verified too; the checksum always is.

  --check               Only report whether an update is available (exit status 10 when one is,
                        0 when up to date)
  --channel <name>      Release channel (default "stable"; reserved for future use)
  --yes, -y             Update without asking
  --require-signature   Refuse to update unless cosign verifies the release signature
  --rollback            Swap the binary with its .bak backup (run again to undo); cannot be
                        combined with other flags
`

// subcommandHelp returns the help text of `build82 <subcommand> --help`, or "" for a name that has
// none.
func subcommandHelp(subcommand string) string {
	switch subcommand {
	case "install":
		return installHelp
	case "uninstall":
		return uninstallHelp
	case "self-update":
		return selfUpdateHelp
	}
	return ""
}

// wantsHelp reports whether `args` asks for help with -h or --help before any "--", after which
// every argument is positional.
func wantsHelp(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "-h", "--help":
			return true
		}
	}
	return false
}

// usageExitCode is the process exit status for command-line usage errors: unknown flags, missing
// flag values and unexpected arguments.
const usageExitCode = 2

// flagValue returns the value following the flag at args[i] and the index of that value. It
// returns an error when the flag is the last argument.
func flagValue(args []string, i int) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("%s requires a value", args[i])
	}
	return args[i+1], i + 1, nil
}

// removedHTTPFlags are the flags of the HTTP mode, removed in v1.2.0; they get an explanatory
// usage error instead of a bare "unknown argument".
var removedHTTPFlags = map[string]bool{"--http": true, "--port": true, "--host": true, "--token": true, "--allowed-host": true}

// parseServeArgs checks the arguments of the server mode, which runs over stdio and takes no flag
// besides --help/-h and --version (they print their output and terminate the process with status
// 0). A "--" ends the arguments; the server takes no positional argument, so anything after it is
// an error. Any other first argument is an error, with a note for the flags of the removed HTTP
// mode (removedHTTPFlags); since every case ends the check, only the first argument is examined.
func parseServeArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch arg := args[0]; arg {
	case "--":
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %q", args[1])
		}
		return nil
	case "--help", "-h":
		fmt.Print(helpText)
		os.Exit(0)
	case "--version":
		fmt.Println(version.Current)
		os.Exit(0)
	}
	if removedHTTPFlags[args[0]] {
		return fmt.Errorf("unknown argument %q: the HTTP mode was removed in v1.2.0; build82 runs over stdio only", args[0])
	}
	return fmt.Errorf("unknown argument %q", args[0])
}

// parseSelfUpdateFlags parses the flags of `build82 self-update` (--check, --yes/-y,
// --require-signature, --channel, --rollback) from `args` and returns them as run options,
// defaulting the channel to "stable", plus whether --rollback was given. A "--" ends the flags;
// self-update takes no positional argument. It returns an error for an
// unknown argument, a --channel without a value, --rollback combined with any other flag, or
// --require-signature combined with --check, which downloads nothing.
func parseSelfUpdateFlags(args []string) (opts selfupdate.RunOptions, rollback bool, err error) {
	opts = selfupdate.RunOptions{Channel: "stable"}
	var others []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			if i+1 < len(args) {
				return opts, false, fmt.Errorf("unexpected argument %q for self-update", args[i+1])
			}
			break
		}
		switch args[i] {
		case "--check":
			opts.Check = true
			others = append(others, args[i])
		case "--yes", "-y":
			opts.Yes = true
			others = append(others, args[i])
		case "--require-signature":
			opts.RequireSignature = true
			others = append(others, args[i])
		case "--rollback":
			rollback = true
		case "--channel":
			others = append(others, args[i])
			if opts.Channel, i, err = flagValue(args, i); err != nil {
				return opts, false, err
			}
		default:
			return opts, false, fmt.Errorf("unknown argument %q for self-update", args[i])
		}
	}
	if rollback && len(others) > 0 {
		return opts, false, fmt.Errorf("--rollback cannot be combined with %s", others[0])
	}
	if opts.Check && opts.RequireSignature {
		return opts, false, errors.New("--require-signature cannot be combined with --check, which downloads nothing")
	}
	return opts, rollback, nil
}

// parseInstallArgs parses the arguments of `build82 install [target]`. It returns the optional
// target (empty when absent) and an error for any flag or a second positional argument.
func parseInstallArgs(args []string) (string, error) {
	target, _, err := parseTargetArgs("install", args, false)
	return target, err
}

// parseUninstallArgs parses the arguments of `build82 uninstall [target] [--purge]`. It returns
// the target (empty when absent) and whether --purge, which may appear anywhere, was given. It
// returns an error for an unknown flag or a second positional argument.
func parseUninstallArgs(args []string) (target string, purge bool, err error) {
	return parseTargetArgs("uninstall", args, true)
}

// parseTargetArgs implements the shared argument walk of install and uninstall: at most one
// positional target, plus --purge when `allowPurge` is set. After "--" every argument is
// positional, even one starting with "-".
func parseTargetArgs(cmd string, args []string, allowPurge bool) (target string, purge bool, err error) {
	positionalOnly := false
	for _, a := range args {
		switch {
		case !positionalOnly && a == "--":
			positionalOnly = true
		case !positionalOnly && allowPurge && a == "--purge":
			purge = true
		case !positionalOnly && strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown argument %q for %s", a, cmd)
		case target == "":
			target = a
		default:
			return "", false, fmt.Errorf("unexpected argument %q for %s", a, cmd)
		}
	}
	return target, purge, nil
}

// exitUsage prints `err` to stderr as a usage error with a pointer to the help of `subcommand`
// (`build82 <subcommand> --help`, or `build82 --help` when it is empty) and exits the process with
// status usageExitCode. It does nothing when `err` is nil.
func exitUsage(subcommand string, err error) {
	if err == nil {
		return
	}
	help := "build82 --help"
	if subcommand != "" {
		help = "build82 " + subcommand + " --help"
	}
	fmt.Fprintf(os.Stderr, "Error: %v\nRun '%s' for usage.\n", err, help)
	os.Exit(usageExitCode)
}

// exitOnError prints `err` to stderr and exits the process with status 1: as
// "Interrupted; nothing was changed." when a prompt was interrupted (prompt.ErrInterrupted),
// otherwise as "Error: ...". It does nothing when `err` is nil. It is shared by the install,
// self-update and uninstall subcommands.
func exitOnError(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, prompt.ErrInterrupted) {
		fmt.Fprintln(os.Stderr, "Interrupted; nothing was changed.")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

// interruptContext returns a context cancelled by the first SIGINT or SIGTERM, so a subcommand
// waiting at a prompt or on the network stops cleanly. After that first signal the default
// handling is restored, so a second Ctrl-C ends the process at once.
func interruptContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx
}

// main dispatches on the first argument: no arguments start the MCP server over stdio, the
// install, self-update and uninstall
// subcommands run their own flows (or print their own help with -h/--help before any "--"),
// --help and --version print and return, and any other bareword is rejected as an unknown
// command with exit status 1. Unknown flags, missing flag values and unexpected arguments are
// usage errors that exit with status 2. A Ctrl-C at a subcommand prompt exits with status 1.
func main() {
	if len(os.Args) < 2 {
		runServe(nil)
		return
	}

	if help := subcommandHelp(os.Args[1]); help != "" && wantsHelp(os.Args[2:]) {
		fmt.Print(help)
		return
	}

	switch os.Args[1] {
	case "install":
		target, err := parseInstallArgs(os.Args[2:])
		exitUsage("install", err)
		exitOnError(installer.Run(interruptContext(), target))
	case "self-update":
		opts, rollback, err := parseSelfUpdateFlags(os.Args[2:])
		exitUsage("self-update", err)
		if rollback {
			runRollback()
			return
		}
		updateAvailable, err := selfupdate.Run(interruptContext(), opts)
		exitOnError(err)
		if updateAvailable {
			os.Exit(selfupdate.ExitUpdateAvailable)
		}
	case "uninstall":
		target, purge, err := parseUninstallArgs(os.Args[2:])
		exitUsage("uninstall", err)
		exitOnError(installer.Uninstall(interruptContext(), target, purge))
	case "--help", "-h":
		fmt.Print(helpText)
	case "--version":
		fmt.Println(version.Current)
	default:
		if strings.HasPrefix(os.Args[1], "-") {
			// Any other flag is checked as a server argument; unknown flags (including those of the
			// removed HTTP mode) are usage errors.
			runServe(os.Args[1:])
			return
		}
		// A bareword that isn't a known subcommand — almost always a typo (e.g. `build82
		// instal`). Starting the stdio MCP server here would hang forever waiting for a JSON-RPC
		// client, so it is reported as an unknown command instead.
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, helpText)
		os.Exit(1)
	}
}

// runRollback implements `build82 self-update --rollback`. It resolves the running binary's path
// with binpath.Resolve and passes it to selfupdate.Rollback, which swaps the binary with the ".bak"
// backup left by a self-update. Failures are reported through exitOnError; success prints the
// restored version and where the replaced binary is kept.
func runRollback() {
	binaryPath, err := binpath.Resolve()
	if err != nil {
		exitOnError(err)
		return
	}
	restored, err := selfupdate.Rollback(binaryPath)
	if err != nil {
		exitOnError(err)
		return
	}
	fmt.Printf("✅ Rolled back to %s at %s (the replaced version is kept at %s.bak; run --rollback again to undo).\n", restored, binaryPath, binaryPath)
}

// runServe checks the server arguments in `args` (parseServeArgs) and runs the MCP server over
// stdio. It returns only when the server stops.
func runServe(args []string) {
	exitUsage("", parseServeArgs(args))
	runStdio()
}

// stdioMaxLineLength bounds the size in bytes of one inbound JSON-RPC message on stdio, well above
// the SDK default so a large request (such as a long plugin list for plugin_batch) is accepted.
const stdioMaxLineLength = 64 << 20

// runStdio serves MCP over stdin/stdout until the client disconnects, exiting with status 1 on a
// fatal error. The startup line goes to stderr, because stdout carries the protocol stream, and
// is printed before the blocking Run call.
func runStdio() {
	fmt.Fprintln(os.Stderr, "build82 server running on stdio")

	srv := server.NewServer()
	if err := srv.Run(context.Background(), &mcp.StdioTransport{MaxLineLength: stdioMaxLineLength}); err != nil {
		fmt.Fprintln(os.Stderr, "Fatal error:", err)
		os.Exit(1)
	}
}
