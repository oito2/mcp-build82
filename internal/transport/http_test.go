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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServerFactory() (*mcp.Server, error) {
	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil), nil
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestWithAuth_DisabledWhenTokenEmpty(t *testing.T) {
	h := withAuth("", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with auth disabled, got %d", rec.Code)
	}
}

func TestWithAuth_RejectsMissingHeader(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no Authorization header, got %d", rec.Code)
	}
}

func TestWithAuth_RejectsWrongToken(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", rec.Code)
	}
}

func TestWithAuth_RejectsDifferentLengthToken(t *testing.T) {
	// Exercises the explicit length check before ConstantTimeCompare.
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer short")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with a different-length token, got %d", rec.Code)
	}
}

func TestWithAuth_AcceptsCorrectToken(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with the correct token, got %d", rec.Code)
	}
}

func TestWithHostValidation_AllowsLocalhostVariants(t *testing.T) {
	h := withHostValidation(nil, "127.0.0.1", okHandler())
	for _, host := range []string{"localhost:8080", "127.0.0.1:8080", "[::1]:8080"} {
		req := httptest.NewRequest("GET", "/mcp", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: expected 200, got %d", host, rec.Code)
		}
	}
}

func TestWithHostValidation_RejectsUnlistedHost(t *testing.T) {
	h := withHostValidation(nil, "127.0.0.1", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for an unlisted host, got %d", rec.Code)
	}
}

func TestWithHostValidation_AllowsCustomAllowedHost(t *testing.T) {
	h := withHostValidation([]string{"my.internal.host"}, "127.0.0.1", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Host = "my.internal.host:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for a custom --allowed-host, got %d", rec.Code)
	}
}

func TestWithHostValidation_HealthBypassesHostCheck(t *testing.T) {
	h := withHostValidation(nil, "127.0.0.1", okHandler())
	req := httptest.NewRequest("GET", "/health", nil)
	req.Host = "some-random-host.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected /health to bypass host validation, got %d", rec.Code)
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestNotFoundHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()
	notFoundHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestIsLoopbackHost verifies the isLoopbackHost helper: only these three
// values (case-insensitively) may be treated as "safe to run without a token" — anything else,
// including a wildcard bind address like 0.0.0.0, must be flagged as network-exposed.
func TestIsLoopbackHost(t *testing.T) {
	loopback := []string{"127.0.0.1", "localhost", "::1", "LOCALHOST", "Localhost"}
	for _, h := range loopback {
		if !isLoopbackHost(h) {
			t.Errorf("expected %q to be considered loopback", h)
		}
	}
	notLoopback := []string{"0.0.0.0", "192.168.1.5", "example.com", ""}
	for _, h := range notLoopback {
		if isLoopbackHost(h) {
			t.Errorf("expected %q to NOT be considered loopback", h)
		}
	}
}

// TestWarnIfInsecure_TokenSetNoWarning confirms the common, correctly-configured case (a non-empty
// token) produces no warning at all.
func TestWarnIfInsecure_TokenSetNoWarning(t *testing.T) {
	var buf bytes.Buffer
	warnIfInsecure(&buf, "secret123", "0.0.0.0")
	if buf.Len() != 0 {
		t.Errorf("expected no warning when a token is set, got:\n%s", buf.String())
	}
}

// TestWarnIfInsecure_EmptyTokenLoopbackWarnsOnce verifies that running --http with
// no token must print a clear warning that /mcp and /sse are
// unauthenticated, even on loopback — but the stronger "exposed to the network" warning should NOT
// fire for a loopback-only bind.
func TestWarnIfInsecure_EmptyTokenLoopbackWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	warnIfInsecure(&buf, "", "127.0.0.1")
	out := buf.String()
	if !strings.Contains(out, "UNAUTHENTICATED") {
		t.Errorf("expected an UNAUTHENTICATED warning, got:\n%s", out)
	}
	if strings.Contains(out, "not loopback") {
		t.Errorf("expected no network-exposure warning for a loopback host, got:\n%s", out)
	}
}

// TestWarnIfInsecure_EmptyTokenNonLoopbackWarnsTwice verifies that no token AND a non-loopback
// bind host (e.g. --host 0.0.0.0) fires both warnings.
func TestWarnIfInsecure_EmptyTokenNonLoopbackWarnsTwice(t *testing.T) {
	var buf bytes.Buffer
	warnIfInsecure(&buf, "", "0.0.0.0")
	out := buf.String()
	if !strings.Contains(out, "UNAUTHENTICATED") {
		t.Errorf("expected an UNAUTHENTICATED warning, got:\n%s", out)
	}
	if !strings.Contains(out, "not loopback") {
		t.Errorf("expected a network-exposure warning for a non-loopback host, got:\n%s", out)
	}
}

// TestNewHTTPServer_TimeoutsConfigured verifies that newHTTPServer sets ReadHeaderTimeout and
// IdleTimeout (protection against Slowloris-style slow clients) by inspecting the *http.Server
// fields directly.
//
// WriteTimeout is asserted to be exactly 0 (unset): net/http's WriteTimeout covers the whole
// connection lifetime, and this server's /sse and /mcp (Streamable HTTP) endpoints intentionally
// keep connections open far longer than any fixed value could safely allow.
func TestNewHTTPServer_TimeoutsConfigured(t *testing.T) {
	srv := newHTTPServer(context.Background(), "127.0.0.1:0", okHandler())

	const wantReadHeaderTimeout = 10 * time.Second
	if srv.ReadHeaderTimeout != wantReadHeaderTimeout {
		t.Errorf("expected ReadHeaderTimeout %v, got %v", wantReadHeaderTimeout, srv.ReadHeaderTimeout)
	}

	const wantIdleTimeout = 120 * time.Second
	if srv.IdleTimeout != wantIdleTimeout {
		t.Errorf("expected IdleTimeout %v, got %v", wantIdleTimeout, srv.IdleTimeout)
	}

	if srv.WriteTimeout != 0 {
		t.Errorf("expected WriteTimeout to be deliberately unset (0), got %v — a fixed WriteTimeout would sever long-lived SSE/Streamable HTTP connections", srv.WriteTimeout)
	}
}

// TestStartHTTPServer_ReadHeaderTimeoutClosesSlowClient is a behavioral end-to-end test: it starts
// a real server via StartHTTPServer on a discovered free port (net.Listen on :0, then release),
// opens a raw TCP
// connection, and sends only a partial request line (never completing the headers with the trailing
// blank line) to simulate a Slowloris-style slow client. Without ReadHeaderTimeout, net/http would
// wait for the rest of the headers indefinitely; with it configured, the server must close the
// connection on its own within roughly ReadHeaderTimeout, observed here as the peer read returning
// (EOF or reset) well before an overly generous outer bound.
func TestStartHTTPServer_ReadHeaderTimeoutClosesSlowClient(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release the probe listener: %v", err)
	}

	cleanup, err := StartHTTPServer(context.Background(), HTTPServerOptions{
		Host:          "127.0.0.1",
		Port:          port,
		ServerFactory: testServerFactory,
	})
	if err != nil {
		t.Fatalf("StartHTTPServer failed to start: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cleanup(shutdownCtx)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial the server: %v", err)
	}
	defer conn.Close()

	// Send an incomplete request: a request line with no headers and, crucially, no terminating
	// "\r\n\r\n" — net/http keeps waiting for more header bytes until ReadHeaderTimeout fires.
	if _, err := conn.Write([]byte("GET /health HTTP/1.1\r\n")); err != nil {
		t.Fatalf("failed to write the partial request: %v", err)
	}

	// ReadHeaderTimeout is 10s; give a generous outer bound so this isn't flaky under load, while
	// still failing loudly if the server never enforces any timeout at all (in which case this read
	// would block until the test binary's own timeout killed it).
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	start := time.Now()
	buf := make([]byte, 16)
	n, readErr := conn.Read(buf)
	elapsed := time.Since(start)

	if readErr == nil {
		t.Fatalf("expected the server to close the connection after ReadHeaderTimeout, but read %d bytes with no error", n)
	}
	if elapsed > 25*time.Second {
		t.Errorf("expected the connection to be closed close to the 10s ReadHeaderTimeout, took %v", elapsed)
	}
}

// TestStartHTTPServer_BindErrorReturnsSynchronously verifies that a bind failure (port already in
// use, permission denied) is returned synchronously from StartHTTPServer. It occupies a port first,
// then asks StartHTTPServer to bind the very same port and requires the error to come back from the
// call itself (not via a channel, callback, or a background goroutine).
func TestStartHTTPServer_BindErrorReturnsSynchronously(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to occupy a port for the test: %v", err)
	}
	defer occupied.Close()

	port := occupied.Addr().(*net.TCPAddr).Port

	cleanup, err := StartHTTPServer(context.Background(), HTTPServerOptions{
		Host:          "127.0.0.1",
		Port:          port,
		ServerFactory: testServerFactory,
	})
	if err == nil {
		if cleanup != nil {
			_ = cleanup(context.Background())
		}
		t.Fatal("expected StartHTTPServer to return an error synchronously when the port is already bound, got nil")
	}
	if cleanup != nil {
		t.Error("expected a nil cleanup function alongside a bind error")
	}
}

// TestStartHTTPServer_ShutdownTimeoutReturnsWithLongLivedConnection verifies that the returned
// cleanup function honors its context deadline. net/http.Server.Shutdown blocks until all active
// connections become idle, and this server's /sse endpoint intentionally holds the underlying
// connection open until the request's context is done or the transport is closed. This opens a raw
// SSE connection that is deliberately never closed by the test, then calls
// cleanup with a short-timeout context and requires it to return promptly with a deadline error
// instead of blocking for the test's (or the process's) lifetime.
func TestStartHTTPServer_ShutdownTimeoutReturnsWithLongLivedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release the probe listener: %v", err)
	}

	cleanup, err := StartHTTPServer(context.Background(), HTTPServerOptions{
		Host:          "127.0.0.1",
		Port:          port,
		ServerFactory: testServerFactory,
	})
	if err != nil {
		t.Fatalf("StartHTTPServer failed to start: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial the SSE endpoint: %v", err)
	}
	defer conn.Close()

	req := fmt.Sprintf("GET /sse HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\nConnection: keep-alive\r\n\r\n", addr)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("failed to write the SSE request: %v", err)
	}

	// Read the response headers plus the initial "endpoint" SSE event to confirm the GET is being
	// held open by the handler (not merely still in flight), then deliberately stop reading and
	// leave the connection open — this is the long-lived, never-closed connection the test needs.
	reader := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read the SSE response headers: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from the SSE endpoint, got %d", resp.StatusCode)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("failed to read the initial SSE event: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{}) // clear the deadline; we're done reading for this test

	const shutdownTimeout = 500 * time.Millisecond
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- cleanup(shutdownCtx) }()

	select {
	case shutdownErr := <-done:
		elapsed := time.Since(start)
		if shutdownErr == nil {
			t.Fatal("expected Shutdown to return a deadline error given the still-open SSE connection, got nil")
		}
		// Generous upper bound: it must return close to the requested timeout, not hang
		// indefinitely (i.e. not for as long as the still-open SSE connection would otherwise
		// require, which is never, since the test intentionally never closes it).
		if elapsed > shutdownTimeout+5*time.Second {
			t.Errorf("expected Shutdown to return within roughly %v, took %v", shutdownTimeout, elapsed)
		}
	case <-time.After(shutdownTimeout + 10*time.Second):
		t.Fatal("Shutdown did not return within a bounded time — it appears to have hung on the open SSE connection")
	}
}
