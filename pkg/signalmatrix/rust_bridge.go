package signalmatrix

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "phantom/gen"
)

// RustBridge calls the Rust compute gRPC server.
type RustBridge struct {
	client pb.ComputeServiceClient
}

func NewRustBridge(addr string) (*RustBridge, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &RustBridge{client: pb.NewComputeServiceClient(conn)}, nil
}

func (rb *RustBridge) PercentChanges(ctx context.Context, prices []float64) ([]float64, error) {
	res, err := rb.client.PercentChanges(ctx, &pb.PercentChangesRequest{Prices: prices})
	if err != nil {
		return nil, err
	}
	return res.Changes, nil
}

func (rb *RustBridge) BuildWindow(ctx context.Context, allPrices []float64, t0Index, estDays, eventHours uint32) ([]float64, error) {
	res, err := rb.client.BuildWindow(ctx, &pb.BuildWindowRequest{
		AllPrices:  allPrices,
		T0Index:    t0Index,
		EstDays:    estDays,
		EventHours: eventHours,
	})
	if err != nil {
		return nil, err
	}
	return res.Window, nil
}

func (rb *RustBridge) AbnormalReturn(ctx context.Context, actual, market []float64, alpha, beta float64) ([]float64, error) {
	res, err := rb.client.AbnormalReturn(ctx, &pb.AbnormalReturnRequest{
		ActualReturns: actual,
		MarketReturns: market,
		Alpha:         alpha,
		Beta:          beta,
	})
	if err != nil {
		return nil, err
	}
	return res.Ar, nil
}

func (rb *RustBridge) CumulativeAbnormalReturn(ctx context.Context, ar []float64) (float64, error) {
	res, err := rb.client.CumulativeAbnormalReturn(ctx, &pb.CARRequest{Ar: ar})
	if err != nil {
		return 0, err
	}
	return res.Car, nil
}

func (rb *RustBridge) TTestOneSample(ctx context.Context, samples []float64) (float64, error) {
	res, err := rb.client.TTestOneSample(ctx, &pb.TTestRequest{Samples: samples})
	if err != nil {
		return 0, err
	}
	return res.TStatistic, nil
}
