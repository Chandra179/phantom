package signalmatrix

import (
	"context"
	"net"
	"testing"

	pb "phantom/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockComputeServer is a hand-rolled test double for pb.ComputeServiceServer.
type mockComputeServer struct {
	pb.UnimplementedComputeServiceServer
}

func (m *mockComputeServer) PercentChanges(_ context.Context, _ *pb.PercentChangesRequest) (*pb.PercentChangesResponse, error) {
	return &pb.PercentChangesResponse{Changes: []float64{0.693, 0.693}}, nil
}

func (m *mockComputeServer) AbnormalReturn(_ context.Context, _ *pb.AbnormalReturnRequest) (*pb.AbnormalReturnResponse, error) {
	return &pb.AbnormalReturnResponse{Ar: []float64{0.03, 0.02}}, nil
}

func (m *mockComputeServer) CumulativeAbnormalReturn(_ context.Context, _ *pb.CARRequest) (*pb.CARResponse, error) {
	return &pb.CARResponse{Car: 0.05}, nil
}

func (m *mockComputeServer) TTestOneSample(_ context.Context, _ *pb.TTestRequest) (*pb.TTestResponse, error) {
	return &pb.TTestResponse{TStatistic: 3.464}, nil
}

// startMockServer starts a gRPC server on an ephemeral port and returns the address.
func startMockServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterComputeServiceServer(srv, &mockComputeServer{})

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Server stopped — expected during cleanup
		}
	}()

	t.Cleanup(func() {
		srv.GracefulStop()
	})

	return lis.Addr().String()
}

func newTestBridge(t *testing.T, addr string) *RustBridge {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &RustBridge{client: pb.NewComputeServiceClient(conn)}
}

func TestRustBridge_PercentChanges(t *testing.T) {
	addr := startMockServer(t)
	bridge := newTestBridge(t, addr)

	result, err := bridge.PercentChanges(context.Background(), []float64{1.0, 2.0, 4.0})
	if err != nil {
		t.Fatalf("PercentChanges: %v", err)
	}

	want := []float64{0.693, 0.693}
	if len(result) != len(want) {
		t.Fatalf("expected len %d, got %d", len(want), len(result))
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %f, want %f", i, result[i], v)
		}
	}
}

func TestRustBridge_AbnormalReturn(t *testing.T) {
	addr := startMockServer(t)
	bridge := newTestBridge(t, addr)

	result, err := bridge.AbnormalReturn(context.Background(), []float64{0.05, 0.03}, []float64{0.02, 0.01}, 0.0, 1.0)
	if err != nil {
		t.Fatalf("AbnormalReturn: %v", err)
	}

	want := []float64{0.03, 0.02}
	if len(result) != len(want) {
		t.Fatalf("expected len %d, got %d", len(want), len(result))
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %f, want %f", i, result[i], v)
		}
	}
}

func TestRustBridge_CumulativeAbnormalReturn(t *testing.T) {
	addr := startMockServer(t)
	bridge := newTestBridge(t, addr)

	result, err := bridge.CumulativeAbnormalReturn(context.Background(), []float64{0.01, 0.02, 0.02})
	if err != nil {
		t.Fatalf("CumulativeAbnormalReturn: %v", err)
	}

	if result != 0.05 {
		t.Errorf("expected 0.05, got %f", result)
	}
}

func TestRustBridge_TTestOneSample(t *testing.T) {
	addr := startMockServer(t)
	bridge := newTestBridge(t, addr)

	result, err := bridge.TTestOneSample(context.Background(), []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("TTestOneSample: %v", err)
	}

	if result != 3.464 {
		t.Errorf("expected 3.464, got %f", result)
	}
}
