package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/config"
)

func testConfig() config.Config {
	return config.Config{
		Port:                    "0",
		ServerReadHeaderTimeout: config.DefaultServerReadHeaderTimeout,
		ServerReadTimeout:       config.DefaultServerReadTimeout,
		ServerWriteTimeout:      config.DefaultServerWriteTimeout,
		ServerIdleTimeout:       config.DefaultServerIdleTimeout,
		ServerShutdownTimeout:   config.DefaultServerShutdownTimeout,
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

// blockingInterceptor holds every unary call until release is closed or the call's
// context ends, and reports on entered when a call has arrived.
func blockingInterceptor(entered chan<- struct{}, release <-chan struct{}) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return handler(ctx, req)
	}
}

func newHealthGRPCServer(opts ...grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)
	return server
}

type lifecycle struct {
	cancel   context.CancelFunc
	done     chan error
	httpAddr string
	grpcAddr string
}

// startLifecycle runs serveUntilDone on fresh loopback listeners.
func startLifecycle(t *testing.T, handler http.Handler, grpcServer *grpc.Server, shutdownTimeout time.Duration) *lifecycle {
	t.Helper()
	httpListener, grpcListener := listen(t), listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	l := &lifecycle{cancel: cancel, done: make(chan error, 1), httpAddr: httpListener.Addr().String(), grpcAddr: grpcListener.Addr().String()}
	httpServer := newHTTPServer(testConfig(), handler)
	go func() {
		l.done <- serveUntilDone(ctx, httpServer, httpListener, grpcServer, grpcListener, shutdownTimeout)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-l.done:
		case <-time.After(5 * time.Second):
			t.Error("serveUntilDone did not return during cleanup")
		}
	})
	return l
}

// signal simulates SIGINT/SIGTERM by cancelling the lifecycle context.
func (l *lifecycle) signal() { l.cancel() }

func (l *lifecycle) wait(t *testing.T, bound time.Duration) error {
	t.Helper()
	select {
	case err := <-l.done:
		l.done <- err
		return err
	case <-time.After(bound):
		t.Fatalf("serveUntilDone did not return within %s of the shutdown signal", bound)
		return nil
	}
}

// waitRefused polls until a new TCP connection to addr is refused.
func waitRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("new connections to %s were still accepted after shutdown began", addr)
}

func TestNewHTTPServerAppliesConfiguredTimeouts(t *testing.T) {
	cfg := config.Config{Port: "8080", ServerReadHeaderTimeout: 1, ServerReadTimeout: 2, ServerWriteTimeout: 3, ServerIdleTimeout: 4}
	server := newHTTPServer(cfg, http.NotFoundHandler())
	if server.Addr != "0.0.0.0:8080" {
		t.Fatalf("Addr=%q, want 0.0.0.0:8080 (port binding must not change)", server.Addr)
	}
	if server.ReadHeaderTimeout != time.Second || server.ReadTimeout != 2*time.Second || server.WriteTimeout != 3*time.Second || server.IdleTimeout != 4*time.Second {
		t.Fatalf("timeouts header=%s read=%s write=%s idle=%s", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}

func TestShutdownCompletesInFlightHTTPRequestAndRefusesNewConnections(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = io.WriteString(w, "done")
	})
	l := startLifecycle(t, handler, newHealthGRPCServer(), 5*time.Second)

	type result struct {
		status int
		body   string
		err    error
	}
	responses := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + l.httpAddr + "/slow")
		if err != nil {
			responses <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		responses <- result{status: resp.StatusCode, body: string(body), err: err}
	}()
	<-entered

	l.signal()
	waitRefused(t, l.httpAddr)
	waitRefused(t, l.grpcAddr)
	select {
	case err := <-l.done:
		t.Fatalf("serveUntilDone returned %v while a request was still in flight", err)
	default:
	}

	close(release)
	got := <-responses
	if got.err != nil || got.status != http.StatusOK || got.body != "done" {
		t.Fatalf("in-flight request status=%d body=%q err=%v, want 200 done", got.status, got.body, got.err)
	}
	// The drain ends well inside the 5s shutdown timeout, so the process exits early.
	if err := l.wait(t, 2*time.Second); err != nil {
		t.Fatalf("graceful shutdown returned %v, want nil", err)
	}
}

func TestShutdownForceClosesHTTPRequestThatOutlivesTimeout(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
	})
	const shutdownTimeout = 300 * time.Millisecond
	l := startLifecycle(t, handler, newHealthGRPCServer(), shutdownTimeout)

	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + l.httpAddr + "/stuck")
		if err == nil {
			resp.Body.Close()
		}
		clientErr <- err
	}()
	<-entered

	start := time.Now()
	l.signal()
	err := l.wait(t, 3*time.Second)
	if err == nil || !strings.Contains(err.Error(), "HTTP shutdown did not finish in time") {
		t.Fatalf("shutdown error=%v, want the HTTP shutdown timeout", err)
	}
	if elapsed := time.Since(start); elapsed < shutdownTimeout || elapsed > 2*time.Second {
		t.Fatalf("shutdown took %s, want about %s", elapsed, shutdownTimeout)
	}
	if err := <-clientErr; err == nil {
		t.Fatal("the stuck request succeeded, want its connection closed")
	}
}

func TestReadHeaderTimeoutDisconnectsIncompleteHeaders(t *testing.T) {
	cfg := testConfig()
	cfg.ServerReadHeaderTimeout = 1
	server := newHTTPServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	listener := listen(t)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	// A complete request on the same server is answered normally.
	resp, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete request status=%d, want 200", resp.StatusCode)
	}

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	// The blank line that ends the header block is never sent.
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: identity\r\n"); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	elapsed := time.Since(start)
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("connection still open after %s, want it closed after ReadHeaderTimeout", elapsed)
	}
	if err == nil && !strings.Contains(line, "408") {
		t.Fatalf("server answered %q to an incomplete request", line)
	}
	if elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("connection closed after %s, want about 1s (ReadHeaderTimeout)", elapsed)
	}
}

func TestGracefulStopCompletesInFlightGRPCCall(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	grpcServer := newHealthGRPCServer(grpc.UnaryInterceptor(blockingInterceptor(entered, release)))
	l := startLifecycle(t, http.NotFoundHandler(), grpcServer, 5*time.Second)

	conn, err := grpc.NewClient(l.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	type result struct {
		status healthpb.HealthCheckResponse_ServingStatus
		err    error
	}
	results := make(chan result, 1)
	go func() {
		resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
		results <- result{status: resp.GetStatus(), err: err}
	}()
	<-entered

	l.signal()
	waitRefused(t, l.grpcAddr)
	waitRefused(t, l.httpAddr)
	select {
	case err := <-l.done:
		t.Fatalf("serveUntilDone returned %v while a gRPC call was still in flight", err)
	default:
	}

	close(release)
	got := <-results
	if got.err != nil || got.status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("in-flight gRPC call status=%s err=%v, want SERVING", got.status, got.err)
	}
	if err := l.wait(t, 2*time.Second); err != nil {
		t.Fatalf("graceful stop returned %v, want nil", err)
	}
}

func TestGRPCFallsBackToStopAfterShutdownTimeout(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer close(release)
	grpcServer := newHealthGRPCServer(grpc.UnaryInterceptor(blockingInterceptor(entered, release)))
	const shutdownTimeout = 300 * time.Millisecond
	l := startLifecycle(t, http.NotFoundHandler(), grpcServer, shutdownTimeout)

	conn, err := grpc.NewClient(l.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	callErr := make(chan error, 1)
	go func() {
		_, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
		callErr <- err
	}()
	<-entered

	start := time.Now()
	l.signal()
	err = l.wait(t, 3*time.Second)
	if err == nil || !strings.Contains(err.Error(), "gRPC graceful stop did not finish in time") {
		t.Fatalf("shutdown error=%v, want the gRPC forced stop", err)
	}
	if elapsed := time.Since(start); elapsed < shutdownTimeout || elapsed > 2*time.Second {
		t.Fatalf("shutdown took %s, want about %s", elapsed, shutdownTimeout)
	}
	if err := <-callErr; err == nil {
		t.Fatal("the stuck gRPC call succeeded, want it cancelled by Stop")
	}
}

func TestServeFailureStopsBothServers(t *testing.T) {
	httpListener, grpcListener := listen(t), listen(t)
	httpListener.Close()
	done := make(chan error, 1)
	go func() {
		done <- serveUntilDone(context.Background(), newHTTPServer(testConfig(), http.NotFoundHandler()), httpListener, newHealthGRPCServer(), grpcListener, time.Second)
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "serve HTTP") {
			t.Fatalf("error=%v, want the HTTP serve failure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serveUntilDone kept running after the HTTP server failed")
	}
	waitRefused(t, grpcListener.Addr().String())
}
