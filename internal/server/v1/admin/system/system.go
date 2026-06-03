package system

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap/request"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
)

type Server struct {
	logger            *slog.Logger
	bucket            *dynamitedb.Bucket
	requestController *request.Controller
}

func New(logger *slog.Logger, bucket *dynamitedb.Bucket, request *request.Controller) *Server {
	return &Server{
		logger:            logger,
		bucket:            bucket,
		requestController: request,
	}
}

func (s *Server) ScanLogs(ctx context.Context, req *connect.Request[system.ScanLogsRequest]) (*connect.Response[system.ScanLogsResponse], error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *Server) AggregateRequests(ctx context.Context, req *connect.Request[system.AggregateRequestsRequest]) (*connect.Response[system.AggregateRequestsResponse], error) {
	windows, err := s.requestController.Aggregate(ctx, time.Now().Add(-time.Hour), time.Now(), 10)
	if err != nil {
		return nil, err
	}
	return &connect.Response[system.AggregateRequestsResponse]{Msg: &system.AggregateRequestsResponse{
		RequestWindows: windows,
	}}, nil
}
