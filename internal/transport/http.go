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

package transport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTPServerOptions configures StartHTTPServer.
type HTTPServerOptions struct {
	Port          int
	Host          string
	Token         string
	AllowedHosts  []string
	ServerFactory func() (*mcp.Server, error)
}

// StartHTTPServer wires the Streamable HTTP and SSE MCP transports behind host-validation and
// Bearer-auth middleware, plus an unauthenticated /health endpoint, and starts listening. Returns
// a shutdown function.
//
// The Go SDK's mcp.NewStreamableHTTPHandler/mcp.NewSSEHandler both implement http.Handler and
// manage sessions internally, so no session-map bookkeeping is needed here. NewSSEHandler also
// handles requests to the message endpoints, so no separate /messages route is registered.
func StartHTTPServer(ctx context.Context, opts HTTPServerOptions) (func(context.Context) error, error) {
	warnIfInsecure(os.Stderr, opts.Token, opts.Host)

	mux := http.NewServeMux()

	getServer := func(*http.Request) *mcp.Server {
		s, err := opts.ServerFactory()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[build82] failed to create server:", err)
			return nil
		}
		return s
	}

	streamableHandler := mcp.NewStreamableHTTPHandler(getServer, nil)
	sseHandler := mcp.NewSSEHandler(getServer, nil)

	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/mcp", withAuth(opts.Token, streamableHandler))
	mux.Handle("/sse", withAuth(opts.Token, sseHandler))
	mux.HandleFunc("/", notFoundHandler)

	handler := withHostValidation(opts.AllowedHosts, opts.Host, mux)

	// The bind (net.Listen) happens synchronously, before this function returns, so a port
	// already in use or a permission error is reported to the caller as a real error
	// instead of only surfacing inside the background goroutine below.
	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	srv := newHTTPServer(ctx, addr, handler)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "[build82] HTTP server error:", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "[build82] HTTP server listening on http://%s:%d\n", opts.Host, opts.Port)
	fmt.Fprintf(os.Stderr, "[build82] MCP endpoint:    http://%s:%d/mcp\n", opts.Host, opts.Port)
	fmt.Fprintf(os.Stderr, "[build82] SSE endpoint:    http://%s:%d/sse\n", opts.Host, opts.Port)

	return srv.Shutdown, nil
}

// newHTTPServer builds the *http.Server used by StartHTTPServer, without binding a socket.
//
// ReadHeaderTimeout and IdleTimeout are set to defend against Slowloris-style attacks, where
// a client opens a connection and trickles bytes slowly enough to tie up a goroutine/file descriptor
// indefinitely:
//   - ReadHeaderTimeout bounds how long the server will wait for a client to finish sending request
//     headers. 10s is generous for any legitimate client (including the SSE/Streamable HTTP clients
//     this server also serves) but rules out an indefinite trickle.
//   - IdleTimeout bounds how long a keep-alive connection may sit between requests before it's
//     closed. 120s comfortably covers normal client reuse without holding sockets open forever.
//
// WriteTimeout is deliberately NOT set. net/http applies WriteTimeout as a deadline covering the
// entire connection lifetime starting at the first byte read (it's reset per-connection, not
// per-request), and this server's /sse and /mcp (Streamable HTTP) endpoints intentionally hold
// connections open far longer than any reasonable fixed value. A fixed WriteTimeout would
// eventually sever those streams. Graceful shutdown (srv.Shutdown) plus the caller's own
// shutdown-timeout context bounds how long any connection can be kept.
func newHTTPServer(ctx context.Context, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
		// Ties request contexts to the caller's ctx (typically cancelled on SIGINT/SIGTERM via
		// signal.NotifyContext) so handlers that watch ctx.Done() can unwind promptly, helping the
		// graceful Shutdown below actually make progress against long-lived SSE/Streamable
		// HTTP connections instead of only relying on the shutdown timeout to force them closed.
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// isLoopbackHost reports whether host is a loopback address that only accepts connections from the
// local machine — used by warnIfInsecure to decide whether running --http without a token
// is a purely local convenience or an actual network exposure.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// warnIfInsecure prints an operator-facing warning to w when --http would run with authentication
// disabled: the registered MCP tools include operations that write to the Moodle disk and run
// builds/zips, so an unauthenticated /mcp and /sse is an attack surface, especially when host
// isn't loopback-only.
func warnIfInsecure(w io.Writer, token, host string) {
	if token != "" {
		return
	}
	fmt.Fprintln(w, "[build82] WARNING: no token set — /mcp and /sse are UNAUTHENTICATED")
	if !isLoopbackHost(host) {
		fmt.Fprintf(w, "[build82] WARNING: host %q is not loopback — the MCP server is exposed to the network with no authentication\n", host)
	}
}

// writeJSON writes body as a JSON response with the given status code, setting the
// Content-Type header first. Used by every handler/middleware in this file that produces a JSON
// body: healthHandler, notFoundHandler, withAuth, and withHostValidation.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error":     "Not Found",
		"endpoints": []string{"/mcp", "/sse", "/health"},
	})
}

// withAuth requires a Bearer token, timing-safe compared, unless token is empty (auth disabled).
func withAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	tokenBytes := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		provided := ""
		if strings.HasPrefix(auth, "Bearer ") {
			provided = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
		if len(provided) != len(tokenBytes) || subtle.ConstantTimeCompare([]byte(provided), tokenBytes) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "Unauthorized",
				"message": "Valid Bearer token required. Set Authorization: Bearer <token> header.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withHostValidation is DNS-rebinding protection: allow-lists localhost, 127.0.0.1, ::1, the bind
// host, and any --allowed-host values. StartHTTPServer wraps every route, including /health, with
// it.
func withHostValidation(allowedHosts []string, bindHost string, next http.Handler) http.Handler {
	allowed := map[string]struct{}{
		"localhost": {}, "127.0.0.1": {}, "::1": {},
	}
	if bindHost != "" {
		allowed[strings.ToLower(bindHost)] = struct{}{}
	}
	for _, h := range allowedHosts {
		allowed[strings.ToLower(h)] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		host := strings.ToLower(hostWithoutPort(r.Host))
		if _, ok := allowed[host]; !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":   "Forbidden",
				"message": fmt.Sprintf("Host %q is not allowed. Pass --allowed-host to permit it.", host),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostWithoutPort strips a trailing ":port" from an HTTP Host header, correctly handling bracketed
// IPv6 literals (e.g. "[::1]:8080" -> "::1"), whose addresses contain colons themselves.
func hostWithoutPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	// No port present (SplitHostPort failed) — the host header had no ":port" suffix at all.
	return strings.Trim(host, "[]")
}
