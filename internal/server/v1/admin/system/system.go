package system

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket
}

func New(logger *slog.Logger, oltp *dynamitedb.Bucket, olap *lake.Bucket) *Server {
	return &Server{
		logger: logger,
		oltp:   oltp,
		olap:   olap,
	}
}

func (s *Server) ScanLogs(ctx context.Context, req *connect.Request[system.ScanLogsRequest]) (*connect.Response[system.ScanLogsResponse], error) {
	levelFilters := []lake.Filter[int64]{}
	if req.Msg.Level != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(req.Msg.Level)); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid level"))
		}
		levelFilters = append(levelFilters, lake.Eq(int64(level)))
	}
	systemFilters := []lake.Filter[string]{}
	if req.Msg.System != "" {
		systemFilters = append(systemFilters, lake.Eq(req.Msg.System))
	}
	procedureFilters := []lake.Filter[string]{}
	if req.Msg.Procedure != "" {
		procedureFilters = append(procedureFilters, lake.Eq(req.Msg.Procedure))
	}
	logs, err := lake.Query[olap.Log]().
		Limit(int(req.Msg.Limit)).
		Where(olap.Log{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			System:    lake.FilterString(systemFilters...),
			Procedure: lake.FilterString(procedureFilters...),
			Level:     lake.FilterInt(levelFilters...),
		}).
		Scan(ctx, s.olap)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	outputLogs := make([]*system.Log, len(logs))
	for i, log := range logs {
		outputLogs[i] = &system.Log{
			Timestamp: timestamppb.New(time.Unix(0, log.Timestamp.Data)),
			Level:     slog.Level(log.Level.Data).String(),
			Message:   log.Message.Data,
			System:    log.System.Data,
			Procedure: log.Procedure.Data,
			Trace:     log.Trace.Data,
		}
	}
	return &connect.Response[system.ScanLogsResponse]{Msg: &system.ScanLogsResponse{
		Logs: outputLogs,
	}}, nil
}

func (s *Server) ScanRequests(ctx context.Context, req *connect.Request[system.ScanRequestsRequest]) (*connect.Response[system.ScanRequestsResponse], error) {
	requests, err := lake.Query[olap.Request]().
		Limit(int(req.Msg.Limit)).
		Where(olap.Request{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Endpoint:  lake.FilterString(lake.Eq(req.Msg.Endpoint)),
		}).
		Scan(ctx, s.olap)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	outputRequests := make([]*system.Request, len(requests))
	for i, log := range requests {
		outputRequests[i] = &system.Request{
			Timestamp: timestamppb.New(time.Unix(log.Timestamp.Data, 0)),
			Endpoint:  log.Endpoint.Data,
			Latency:   log.Latency.Data,
			Source:    log.Source.Data,
		}
	}
	return &connect.Response[system.ScanRequestsResponse]{Msg: &system.ScanRequestsResponse{
		Requests: outputRequests,
	}}, nil
}

func (s *Server) AggregateLatency(ctx context.Context, req *connect.Request[system.AggregateLatencyRequest]) (*connect.Response[system.AggregateLatencyResponse], error) {
	aggrWindow := lake.DateHour
	switch req.Msg.Window {
	case system.AggregateWindow_Minute:
		aggrWindow = lake.DateMinute
	case system.AggregateWindow_Hour:
		aggrWindow = lake.DateHour
	case system.AggregateWindow_Day:
		aggrWindow = lake.DateDay
	case system.AggregateWindow_Month:
		aggrWindow = lake.DateMonth
	}

	windows, err := lake.Query[olap.Request]().
		Where(olap.Request{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Type:      lake.FilterInt(lake.Eq(int64(olap.RequestUnary))),
		}).
		GroupBy(olap.Request{
			Timestamp: lake.GroupInt(lake.Date(aggrWindow)),
			Endpoint:  lake.GroupString(lake.Exact()),
		}).
		Aggregate(olap.Request{
			Latency: lake.AggrInt(lake.Avg),
		}).
		Scan(ctx, s.olap)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	endpoints := map[string]*system.EndpointLatency{}
	for _, window := range windows {
		endpoint, ok := endpoints[window.Endpoint.Data]
		if !ok {
			endpoints[window.Endpoint.Data] = &system.EndpointLatency{
				Endpoint: window.Endpoint.Data,
				Time:     []*timestamppb.Timestamp{timestamppb.New(time.Unix(window.Timestamp.Data, 0))},
				Latency:  []int64{window.Latency.Data},
			}
		} else {
			endpoint.Time = append(endpoint.Time, timestamppb.New(time.Unix(window.Timestamp.Data, 0)))
			endpoint.Latency = append(endpoint.Latency, window.Latency.Data)
		}
	}
	return &connect.Response[system.AggregateLatencyResponse]{Msg: &system.AggregateLatencyResponse{
		Endpoints: endpoints,
	}}, nil
}

func (s *Server) AggregateHits(ctx context.Context, req *connect.Request[system.AggregateHitsRequest]) (*connect.Response[system.AggregateHitsResponse], error) {
	aggrWindow := lake.DateHour
	switch req.Msg.Window {
	case system.AggregateWindow_Minute:
		aggrWindow = lake.DateMinute
	case system.AggregateWindow_Hour:
		aggrWindow = lake.DateHour
	case system.AggregateWindow_Day:
		aggrWindow = lake.DateDay
	case system.AggregateWindow_Month:
		aggrWindow = lake.DateMonth
	}

	windows, err := lake.Query[olap.Request]().
		Where(olap.Request{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Type:      lake.FilterInt(lake.Eq(int64(olap.RequestUnary))),
		}).
		GroupBy(olap.Request{
			Timestamp: lake.GroupInt(lake.Date(aggrWindow)),
			Endpoint:  lake.GroupString(lake.Exact()),
		}).
		Aggregate(olap.Request{
			Type: lake.AggrInt(lake.Count), // for count it doesn't matter what property we use
		}).
		Scan(ctx, s.olap)
	if err != nil {
		return nil, err
	}
	endpoints := map[string]*system.EndpointHits{}
	for _, window := range windows {
		endpoint, ok := endpoints[window.Endpoint.Data]
		if !ok {
			endpoints[window.Endpoint.Data] = &system.EndpointHits{
				Endpoint: window.Endpoint.Data,
				Time:     []*timestamppb.Timestamp{timestamppb.New(time.Unix(window.Timestamp.Data, 0))},
				Count:    []int64{window.Type.Data},
			}
		} else {
			endpoint.Time = append(endpoint.Time, timestamppb.New(time.Unix(window.Timestamp.Data, 0)))
			endpoint.Count = append(endpoint.Count, window.Type.Data)
		}
	}
	return &connect.Response[system.AggregateHitsResponse]{Msg: &system.AggregateHitsResponse{
		Endpoints: endpoints,
	}}, nil
}
