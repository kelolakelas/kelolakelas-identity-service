package grpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RequestLog records RPC outcomes without serializing protobuf messages or metadata.
// Only a short, log-safe correlation ID is accepted from the caller.
func RequestLog(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	id := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get("x-request-id")
		if len(values) == 1 && validRequestID(values[0]) {
			id = values[0]
		}
	}
	start := time.Now()
	response, err := handler(ctx, req)
	slog.InfoContext(ctx, "grpc request", "request_id", id, "method", info.FullMethod, "code", status.Code(err).String(), "duration_ms", time.Since(start).Milliseconds())
	return response, err
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
