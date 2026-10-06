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

// testServerFactory returns an empty MCP server, for tests that do not exercise MCP behavior.
func testServerFactory() (*mcp.Server, error) {
	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil), nil
}

// okHandler returns a handler that answers 200 with an empty body.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestWithAuth_DisabledWhenTokenEmpty verifies that an empty token lets every request through.
func TestWithAuth_DisabledWhenTokenEmpty(t *testing.T) {
	h := withAuth("", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with auth disabled, got %d", rec.Code)
	}
}

// TestWithAuth_RejectsMissingHeader verifies that a request without Authorization gets 401.
func TestWithAuth_RejectsMissingHeader(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no Authorization header, got %d", rec.Code)
	}
}

// TestWithAuth_RejectsWrongToken verifies that a wrong Bearer token of equal length gets 401.
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

// TestWithAuth_RejectsDifferentLengthToken verifies that a Bearer token of a different length gets 401.
func TestWithAuth_RejectsDifferentLengthToken(t *testing.T) {
	// A different length is rejected by the length check that precedes the constant-time comparison.
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer short")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with a different-length token, got %d", rec.Code)
	}
}

// TestWithAuth_AcceptsCorrectToken verifies that the correct Bearer token is accepted.
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

// TestWithHostValidation_AllowsLocalhostVariants verifies that localhost, 127.0.0.1 and [::1] Host
// headers with a port are accepted.
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

// TestWithHostValidation_RejectsUnlistedHost verifies that an unlisted Host header gets 403.
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

// TestWithHostValidation_AllowsCustomAllowedHost verifies that a host passed in the allowed list is accepted.
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

// TestWithHostValidation_HealthBypassesHostCheck verifies that /health is served for any Host header.
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

// TestHealthHandler verifies that the health handler answers 200 with a JSON content type.
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

// TestNotFoundHandler verifies that the fallback handler answers 404.
func TestNotFoundHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()
	notFoundHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestIsLoopbackHost verifies that only 127.0.0.1, localhost and ::1 (case-insensitive) count as
// loopback, and that anything else, including 0.0.0.0 and the empty string, does not.
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

// TestWarnIfInsecure_TokenSetNoWarning verifies that a non-empty token produces no warning.
func TestWarnIfInsecure_TokenSetNoWarning(t *testing.T) {
	var buf bytes.Buffer
	warnIfInsecure(&buf, "secret123", "0.0.0.0")
	if buf.Len() != 0 {
		t.Errorf("expected no warning when a token is set, got:\n%s", buf.String())
	}
}

// TestWarnIfInsecure_EmptyTokenLoopbackWarnsOnce verifies that an empty token on a loopback host
// produces the unauthenticated warning but not the network-exposure warning.
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

// TestWarnIfInsecure_EmptyTokenNonLoopbackWarnsTwice verifies that an empty token on a non-loopback
// host produces both warnings.
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

// TestNewHTTPServer_TimeoutsConfigured verifies the ReadHeaderTimeout and IdleTimeout values of
// the server returned by newHTTPServer, and that WriteTimeout is left unset because it would sever
// the long-lived /sse and /mcp streams.
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

// TestStartHTTPServer_ReadHeaderTimeoutClosesSlowClient verifies that a server started by
// StartHTTPServer closes a connection whose client sends only a partial request line and never
// finishes the headers. The server listens on a free port, and the test expects the client read
// to fail within a generous bound around the 10s ReadHeaderTimeout.
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

	// Send a request line without the terminating blank line, so the server keeps waiting for
	// headers until ReadHeaderTimeout fires.
	if _, err := conn.Write([]byte("GET /health HTTP/1.1\r\n")); err != nil {
		t.Fatalf("failed to write the partial request: %v", err)
	}

	// The timeout is 10s. The 30s deadline avoids flakiness under load and still fails the test
	// when no timeout is enforced.
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

// TestStartHTTPServer_BindErrorReturnsSynchronously verifies that StartHTTPServer returns a bind
// failure from the call itself, with a nil shutdown function. It occupies a port and then asks
// StartHTTPServer to bind the same one.
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

// TestStartHTTPServer_ShutdownTimeoutReturnsWithLongLivedConnection verifies that the shutdown
// function returned by StartHTTPServer honors its context deadline. Graceful shutdown waits for
// open connections, so the test opens an SSE connection and never closes it, calls the function
// with a short timeout and expects a prompt non-nil (deadline) error.
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

	// Read the response headers and the first SSE line to confirm the handler holds the stream
	// open, then stop reading and leave the connection open.
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
	_ = conn.SetReadDeadline(time.Time{}) // clear the deadline

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
		// It must return close to the requested timeout instead of waiting for the open connection.
		if elapsed > shutdownTimeout+5*time.Second {
			t.Errorf("expected Shutdown to return within roughly %v, took %v", shutdownTimeout, elapsed)
		}
	case <-time.After(shutdownTimeout + 10*time.Second):
		t.Fatal("Shutdown did not return within a bounded time — it appears to have hung on the open SSE connection")
	}
}
