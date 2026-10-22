package main

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// A malformed :path must be rejected before path-based authorization runs.
func TestGRPCRejectsNonCanonicalMethod(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { listener.Close() })
	var intercepted atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		intercepted.Add(1)
		return handler(ctx, req)
	}))
	healthpb.RegisterHealthServer(server, health.NewServer())
	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const method = "/grpc.health.v1.Health/Check"
	if err := conn.Invoke(ctx, method, &healthpb.HealthCheckRequest{}, &healthpb.HealthCheckResponse{}); err != nil {
		t.Fatalf("canonical method: %v", err)
	}
	if got := intercepted.Load(); got != 1 {
		t.Fatalf("canonical method reached interceptor %d times, want 1", got)
	}
	err = conn.Invoke(ctx, method[1:], &healthpb.HealthCheckRequest{}, &healthpb.HealthCheckResponse{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("malformed method: got %v, want Unimplemented", err)
	}
	if got := intercepted.Load(); got != 1 {
		t.Fatalf("malformed method reached authorization interceptor: calls=%d", got)
	}
}
