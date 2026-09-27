package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/config"
)

// newHTTPServer applies the configured timeouts to the identity HTTP server. Without
// them a client that stalls while sending headers, a body, or reading a response would
// hold a goroutine and a file descriptor indefinitely.
func newHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              "0.0.0.0:" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(cfg.ServerReadHeaderTimeout) * time.Second,
		ReadTimeout:       time.Duration(cfg.ServerReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.ServerWriteTimeout) * time.Second,
		IdleTimeout:       time.Duration(cfg.ServerIdleTimeout) * time.Second,
	}
}

// serveUntilDone serves HTTP and gRPC on already bound listeners until ctx is cancelled
// (SIGINT/SIGTERM) or either server fails. It then stops both: HTTP stops accepting and
// drains in-flight requests with Shutdown, gRPC drains in-flight calls with GracefulStop.
// Both share one shutdownTimeout; when it elapses the remaining HTTP connections are
// closed and gRPC falls back to Stop, so the process always exits within the bound.
//
// A failure of either server stops the other one too, so identity never keeps running
// with only half of its interfaces.
func serveUntilDone(ctx context.Context, httpServer *http.Server, httpListener net.Listener, grpcServer *grpc.Server, grpcListener net.Listener, shutdownTimeout time.Duration) error {
	serveErrs := make(chan error, 2)
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrs <- fmt.Errorf("serve HTTP: %w", err)
			return
		}
		serveErrs <- nil
	}()
	go func() {
		// Serve returns nil once Stop or GracefulStop has been called, and ErrServerStopped
		// when the stop happened before Serve started (a signal right at startup).
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErrs <- fmt.Errorf("serve gRPC: %w", err)
			return
		}
		serveErrs <- nil
	}()

	var serveErr error
	pending := 2
	select {
	case <-ctx.Done():
		slog.Info("Shutdown signal received, draining HTTP and gRPC", "timeout", shutdownTimeout.String())
	case serveErr = <-serveErrs:
		pending--
		slog.Error("Server stopped unexpectedly, shutting down", "error", serveErr)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErrs := make(chan error, 2)
	go func() { shutdownErrs <- shutdownHTTP(shutdownCtx, httpServer) }()
	go func() { shutdownErrs <- stopGRPC(shutdownCtx, grpcServer) }()
	errs := []error{serveErr}
	for range 2 {
		errs = append(errs, <-shutdownErrs)
	}
	for ; pending > 0; pending-- {
		errs = append(errs, <-serveErrs)
	}
	err := errors.Join(errs...)
	if err == nil {
		slog.Info("Identity service stopped gracefully")
	}
	return err
}

// shutdownHTTP drains in-flight HTTP requests and force-closes whatever is still open
// when ctx expires.
func shutdownHTTP(ctx context.Context, server *http.Server) error {
	if err := server.Shutdown(ctx); err != nil {
		closeErr := server.Close()
		return errors.Join(fmt.Errorf("HTTP shutdown did not finish in time, connections closed: %w", err), closeErr)
	}
	return nil
}

// stopGRPC lets in-flight gRPC calls finish with GracefulStop and falls back to Stop,
// which cancels them, when ctx expires first.
func stopGRPC(ctx context.Context, server *grpc.Server) error {
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		server.Stop()
		<-stopped
		return fmt.Errorf("gRPC graceful stop did not finish in time, forced stop: %w", ctx.Err())
	}
}
