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

// HTTPServerOptions configures StartHTTPServer. Port and Host give the TCP bind address. Token
// is the required Bearer token, and an empty value disables authentication. AllowedHosts lists
// extra Host header values to accept beyond the loopback names and Host. ServerFactory builds the
// MCP server for each new session.
type HTTPServerOptions struct {
	Port          int
	Host          string
	Token         string
	AllowedHosts  []string
	ServerFactory func() (*mcp.Server, error)
}

// StartHTTPServer serves the Streamable HTTP transport at /mcp and the SSE transport at /sse
// (both behind Bearer-auth middleware when `opts.Token` is set), plus an unauthenticated /health
// endpoint, with Host header validation applied in front of the routes. The listener is bound
// before the function returns, and requests are served in a background goroutine whose lifetime
// is tied to `ctx` through the request base context.
//
// It returns a shutdown function that gracefully stops the server, bounded by the context passed
// to it. It returns an error when the address cannot be bound. A warning is printed to stderr
// when authentication is disabled.
//
// The SDK handlers manage sessions internally, and the SSE handler also serves its message
// endpoint, so no session bookkeeping or separate /messages route is needed here.
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

	// Binding synchronously reports a port in use or a permission error to the caller instead of
	// only logging it from the background goroutine.
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

// newHTTPServer builds the http.Server used by StartHTTPServer for `addr` and `handler`, without
// binding a socket. Request contexts derive from `ctx`.
//
// ReadHeaderTimeout and IdleTimeout guard against slow-client attacks that hold connections open:
//   - ReadHeaderTimeout (10s) bounds how long a client may take to send the request headers.
//   - IdleTimeout (120s) bounds how long a keep-alive connection may wait between requests.
//
// WriteTimeout is deliberately not set. It is a deadline on the whole response, and the /sse and
// /mcp endpoints keep streams open for long periods, so a fixed value would sever them. Graceful
// shutdown with a caller-supplied timeout bounds connection lifetime instead.
func newHTTPServer(ctx context.Context, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
		// Request contexts derive from the caller's ctx, so cancelling it lets handlers that watch
		// ctx.Done() unwind and long-lived streams end during graceful shutdown.
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// isLoopbackHost reports whether `host` is one of the literal loopback names ("127.0.0.1",
// "localhost" or "::1", case-insensitive) that only accept local connections. warnIfInsecure
// uses it to tell a local-only setup from network exposure.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// warnIfInsecure writes a warning to `w` when `token` is empty, meaning /mcp and /sse are
// unauthenticated. An additional warning is written when `host` is not loopback, since the
// exposed tools can write to disk and run builds. It writes nothing when a token is set.
func warnIfInsecure(w io.Writer, token, host string) {
	if token != "" {
		return
	}
	fmt.Fprintln(w, "[build82] WARNING: no token set — /mcp and /sse are UNAUTHENTICATED")
	if !isLoopbackHost(host) {
		fmt.Fprintf(w, "[build82] WARNING: host %q is not loopback — the MCP server is exposed to the network with no authentication\n", host)
	}
}

// writeJSON writes `body` as a JSON response with HTTP status `status` and a JSON Content-Type.
// Encoding errors are ignored. Every handler and middleware in this file uses it for its
// responses.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// healthHandler answers with 200 and {"status":"ok"}.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// notFoundHandler answers unmatched routes with 404 and a JSON body listing the valid endpoints.
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error":     "Not Found",
		"endpoints": []string{"/mcp", "/sse", "/health"},
	})
}

// withAuth wraps `next` so that requests must carry an "Authorization: Bearer <token>" header
// matching `token`, compared in constant time. Other requests receive a 401 JSON response. When
// `token` is empty, authentication is disabled and `next` is returned unchanged.
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

// withHostValidation wraps `next` with DNS-rebinding protection. It accepts requests whose Host
// header (port ignored, case-insensitive) is localhost, 127.0.0.1, ::1, `bindHost` or one of
// `allowedHosts`, and answers others with a 403 JSON response. The /health path is exempt from
// the check.
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

// hostWithoutPort returns `host`, an HTTP Host header value, without its ":port" suffix. Bracketed
// IPv6 literals are handled (for example "[::1]:8080" becomes "::1"), and surrounding brackets
// are removed when no port is present.
func hostWithoutPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	// SplitHostPort fails when there is no port suffix.
	return strings.Trim(host, "[]")
}
