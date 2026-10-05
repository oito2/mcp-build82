// Copyright (C) 2026  oito2
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
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/installer"
	"github.com/oito2/mcp-build82/internal/selfupdate"
	"github.com/oito2/mcp-build82/internal/server"
	"github.com/oito2/mcp-build82/internal/transport"
	"github.com/oito2/mcp-build82/internal/version"
)

const helpText = `build82 — MCP server for Moodle plugin development

Usage:
  build82                  Run the MCP server over stdio (default)
  build82 --http           Run the MCP server over Streamable HTTP + SSE
  build82 install [target] Configure build82 as an MCP server in an AI tool
  build82 self-update      Update to the latest released version
  build82 uninstall [target] Remove build82's MCP server registration

Server flags (only apply with --http):
  --port <n>            Port to listen on (default 3000)
  --host <host>          Host to bind to (default 127.0.0.1)
  --token <token>        Require this Bearer token for /mcp and /sse (default: auth disabled).
                         Can also be set via the BUILD82_TOKEN environment variable; --token takes
                         precedence when both are set. Passing --token with an empty value is a
                         configuration error (use neither at all to run without authentication).
  --allowed-host <host>  Additional allowed Host header value (repeatable)

self-update flags:
  --check                Report whether an update is available, without installing it
  --channel <name>        Release channel (default "stable"; reserved for future use)
  --yes, -y               Skip the confirmation prompt before replacing the running binary
  --rollback              Restore the previous binary from its .bak backup and exit; ignores
                          --check/--channel/--yes if also passed

uninstall flags:
  --purge                 Also delete generated files and the config file (asks for its own confirmation)

Flags:
  -h, --help             Show this help text and exit
  --version              Print the version and exit
`

// serveFlags holds the parsed server-mode flags. Parsed with a manual argv walk rather than the
// flag package — deliberate, since flag's automatic -h/--help handling would conflict with this
// command's own subcommand-aware help text.
type serveFlags struct {
	http         bool
	port         int
	host         string
	token        string
	tokenSet     bool // true iff --token was passed on the CLI with a following value (even "")
	allowedHosts []string
}

func parseServeFlags(args []string) serveFlags {
	f := serveFlags{port: 3000, host: "127.0.0.1"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--http":
			f.http = true
		case "--port":
			if i+1 < len(args) {
				i++
				if n, err := strconv.Atoi(args[i]); err == nil && n > 0 && n < 65536 {
					f.port = n
				} else {
					fmt.Fprintf(os.Stderr, "warning: invalid --port value %q, keeping %d\n", args[i], f.port)
				}
			} else {
				fmt.Fprintln(os.Stderr, "warning: --port requires a value, ignoring")
			}
		case "--host":
			if i+1 < len(args) {
				i++
				f.host = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "warning: --host requires a value, ignoring")
			}
		case "--token":
			if i+1 < len(args) {
				i++
				f.token = args[i]
				f.tokenSet = true
			} else {
				fmt.Fprintln(os.Stderr, "warning: --token requires a value, ignoring")
			}
		case "--allowed-host":
			if i+1 < len(args) {
				i++
				f.allowedHosts = append(f.allowedHosts, args[i])
			} else {
				fmt.Fprintln(os.Stderr, "warning: --allowed-host requires a value, ignoring")
			}
		case "--help", "-h":
			fmt.Print(helpText)
			os.Exit(0)
		case "--version":
			fmt.Println(version.Current)
			os.Exit(0)
		}
	}
	return f
}

// parseSelfUpdateFlags parses `build82 self-update`'s own flags — kept separate from
// serveFlags since they're a disjoint set with no overlap.
func parseSelfUpdateFlags(args []string) selfupdate.RunOptions {
	opts := selfupdate.RunOptions{Channel: "stable"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			opts.Check = true
		case "--yes", "-y":
			opts.Yes = true
		case "--channel":
			if i+1 < len(args) {
				i++
				opts.Channel = args[i]
			}
		}
	}
	return opts
}

// hasFlag reports whether flag appears verbatim among args. Used to detect --rollback ahead of
// parseSelfUpdateFlags's normal Check/Yes/Channel parsing — a rollback is a distinct operation
// from the rest of self-update and doesn't go through RunOptions/selfupdate.Run at all.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// parseUninstallArgs parses `build82 uninstall [target] [--purge]` — the first non-flag
// argument is the target ID; --purge may appear anywhere.
func parseUninstallArgs(args []string) (target string, purge bool) {
	for _, a := range args {
		switch {
		case a == "--purge":
			purge = true
		case target == "" && !strings.HasPrefix(a, "-"):
			target = a
		}
	}
	return target, purge
}

// exitOnError prints err to stderr in this command's standard "Error: ..." shape and exits with
// status 1, unless err is nil (a no-op then). Shared by the install/self-update/uninstall
// subcommands.
func exitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		runServe(nil)
		return
	}

	switch os.Args[1] {
	case "--http":
		runServe(os.Args[1:])
	case "install":
		var target string
		if len(os.Args) > 2 {
			target = os.Args[2]
		}
		exitOnError(installer.Run(target))
	case "self-update":
		args := os.Args[2:]
		if hasFlag(args, "--rollback") {
			runRollback()
		} else {
			exitOnError(selfupdate.Run(parseSelfUpdateFlags(args)))
		}
	case "uninstall":
		target, purge := parseUninstallArgs(os.Args[2:])
		exitOnError(installer.Uninstall(target, purge))
	case "--help", "-h":
		fmt.Print(helpText)
	case "--version":
		fmt.Println(version.Current)
	default:
		if strings.HasPrefix(os.Args[1], "-") {
			// Unrecognized flag is treated as server flags (e.g. bare `--port 8080` isn't valid
			// without --http, but keep parsing permissive/consistent with the --http-gated design).
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

// runRollback implements `build82 self-update --rollback`: it resolves the currently running
// binary's path the same way self-update itself locates currentBinaryPath (via binpath.Resolve)
// and hands it to selfupdate.Rollback, which promotes the ".bak" backup left by a previous
// self-update back into place.
func runRollback() {
	binaryPath, err := binpath.Resolve()
	if err != nil {
		exitOnError(err)
		return
	}
	if err := selfupdate.Rollback(binaryPath); err != nil {
		exitOnError(err)
		return
	}
	fmt.Printf("✅ Rolled back to previous version at %s.\n", binaryPath)
}

func runServe(args []string) {
	flags := parseServeFlags(args)
	if !flags.http {
		runStdio()
		return
	}
	runHTTP(flags)
}

func runStdio() {
	// Server.Run blocks until the client disconnects or the context is cancelled, so the startup
	// line is printed before calling it.
	fmt.Fprintln(os.Stderr, "build82 server running on stdio")

	srv := server.NewServer()
	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "Fatal error:", err)
		os.Exit(1)
	}
}

// buildTokenEnvVar is the environment variable fallback for --http's Bearer token — read
// only when --token was not passed at all on the CLI, so an explicit --token (including an
// explicit empty one, caught by resolveToken below) always wins over the environment. Passing the
// token this way instead of on the command line avoids it being visible to other local processes
// via `ps`/`/proc/<pid>/cmdline`/shell history.
const buildTokenEnvVar = "BUILD82_TOKEN"

// resolveToken determines the effective --http Bearer token from the parsed CLI flags and the
// BUILD82_TOKEN environment variable, while distinguishing "no token configured at all"
// (auth intentionally disabled) from "a token source was used but resolved to an empty string"
// — the latter is treated as a configuration error rather than silently disabling auth,
// since it's most often a shell interpolating an empty variable into `--token "$TOKEN"` or a typo'd
// `export BUILD82_TOKEN=`.
//
// The only way to legitimately run with authentication disabled is to omit --token from the CLI
// entirely AND leave BUILD82_TOKEN unset (not merely empty) in the environment.
func resolveToken(flags serveFlags) (string, error) {
	if flags.tokenSet {
		if flags.token == "" {
			return "", fmt.Errorf("--token was passed with an empty value; omit --token entirely to run --http without authentication")
		}
		return flags.token, nil
	}
	if envToken, ok := os.LookupEnv(buildTokenEnvVar); ok {
		if envToken == "" {
			return "", fmt.Errorf("%s is set but empty; unset it entirely to run --http without authentication", buildTokenEnvVar)
		}
		return envToken, nil
	}
	return "", nil
}

// shutdownTimeout bounds how long graceful shutdown waits for in-flight connections —
// notably long-lived SSE/Streamable HTTP sessions — to finish before forcing the listener closed.
const shutdownTimeout = 10 * time.Second

func runHTTP(flags serveFlags) {
	token, err := resolveToken(flags)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cleanup, err := transport.StartHTTPServer(ctx, transport.HTTPServerOptions{
		Port: flags.port, Host: flags.host, Token: token, AllowedHosts: flags.allowedHosts,
		ServerFactory: func() (*mcp.Server, error) { return server.NewServer(), nil },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal error:", err)
		os.Exit(1)
	}

	<-ctx.Done()
	fmt.Fprintln(os.Stderr, "[build82] shutting down...")
	// net/http.Server.Shutdown blocks until all connections are idle, and the SSE/Streamable
	// HTTP transport keeps long-lived connections open — an idle session left connected could hang
	// Shutdown (and thus the whole process's exit on SIGINT/SIGTERM) indefinitely. Bound it with a
	// timeout so the process always terminates, forcibly closing any still-open connections once it
	// expires.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := cleanup(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "[build82] graceful shutdown did not complete within", shutdownTimeout, "-", err)
	}
}
