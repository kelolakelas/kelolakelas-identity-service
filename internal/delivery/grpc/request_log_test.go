package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestRequestLogMetadataIsAllowlisted(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, id := range []string{"trace-1", "bad\ntrace"} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", id, "authorization", "hidden-token"))
		_, err := RequestLog(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/tenant.PermissionService/CheckPermission"}, func(context.Context, any) (any, error) { return nil, errors.New("hidden-error") })
		if err == nil {
			t.Fatal("handler error lost")
		}
	}
	if !strings.Contains(logs.String(), `"request_id":"trace-1"`) || !strings.Contains(logs.String(), `"code":"Unknown"`) {
		t.Fatalf("missing request correlation or status: %s", logs.String())
	}
	for _, secret := range []string{"hidden-token", "hidden-error", "bad\\ntrace"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("sensitive value logged: %s", secret)
		}
	}
}
