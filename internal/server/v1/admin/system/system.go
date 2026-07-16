package system

import (
	"context"
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
	logs, err := lake.Query[olap.Log]().
		Limit(int(req.Msg.Limit)).
		Where(olap.Log{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			System:    lake.FilterString(lake.When(req.Msg.System != "", lake.Eq(req.Msg.System))),
			Procedure: lake.FilterString(lake.When(req.Msg.Procedure != "", lake.Eq(req.Msg.Procedure))),
			Level:     lake.FilterInt(lake.When(req.Msg.Level != "", lake.Eq(int64(getLevel(req.Msg.Level))))),
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

func (s *Server) AggregateLogs(ctx context.Context, req *connect.Request[system.AggregateLogsRequest]) (*connect.Response[system.AggregateLogsResponse], error) {
	windows, err := lake.Query[olap.Log]().
		Where(olap.Log{
			Timestamp:  lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Level:      lake.FilterInt(lake.When(req.Msg.MinLevel != "", lake.Gte(int64(getLevel(req.Msg.MinLevel))))),
			Redirected: lake.FilterInt(lake.Eq(int64(0))),
		}).
		GroupBy(olap.Log{
			Timestamp: lake.GroupInt(lake.Date(getRange(req.Msg.Window))),
			Level:     lake.GroupInt(lake.Exact()),
		}).
		Aggregate(olap.Log{
			Redirected: lake.AggrInt(lake.Count), // for count it doesn't matter what property we use
		}).
		Scan(ctx, s.olap)
	if err != nil {
		return nil, err
	}
	levelCounts := map[string]*system.LevelCount{}
	for _, window := range windows {
		level := slog.Level(window.Level.Data).String()
		levelCount, ok := levelCounts[level]
		if !ok {
			levelCounts[level] = &system.LevelCount{
				Level: level,
				Time:  []*timestamppb.Timestamp{timestamppb.New(time.Unix(0, window.Timestamp.Data))},
				Count: []int64{window.Redirected.Data},
			}
		} else {
			levelCount.Time = append(levelCount.Time, timestamppb.New(time.Unix(0, window.Timestamp.Data)))
			levelCount.Count = append(levelCount.Count, window.Redirected.Data)
		}
	}
	return &connect.Response[system.AggregateLogsResponse]{Msg: &system.AggregateLogsResponse{
		Levels: levelCounts,
	}}, nil
}

func (s *Server) AggregateLatency(ctx context.Context, req *connect.Request[system.AggregateLatencyRequest]) (*connect.Response[system.AggregateLatencyResponse], error) {
	windows, err := lake.Query[olap.Request]().
		Where(olap.Request{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Type:      lake.FilterInt(lake.Eq(int64(olap.RequestUnary))),
		}).
		GroupBy(olap.Request{
			Timestamp: lake.GroupInt(lake.Date(getRange(req.Msg.Window))),
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
				Time:     []*timestamppb.Timestamp{timestamppb.New(time.Unix(0, window.Timestamp.Data))},
				Latency:  []int64{window.Latency.Data},
			}
		} else {
			endpoint.Time = append(endpoint.Time, timestamppb.New(time.Unix(0, window.Timestamp.Data)))
			endpoint.Latency = append(endpoint.Latency, window.Latency.Data)
		}
	}
	return &connect.Response[system.AggregateLatencyResponse]{Msg: &system.AggregateLatencyResponse{
		Endpoints: endpoints,
	}}, nil
}

func (s *Server) AggregateHits(ctx context.Context, req *connect.Request[system.AggregateHitsRequest]) (*connect.Response[system.AggregateHitsResponse], error) {
	windows, err := lake.Query[olap.Request]().
		Where(olap.Request{
			Timestamp: lake.FilterInt(lake.After(req.Msg.From.AsTime()), lake.Before(req.Msg.To.AsTime())),
			Type:      lake.FilterInt(lake.Eq(int64(olap.RequestUnary))),
		}).
		GroupBy(olap.Request{
			Timestamp: lake.GroupInt(lake.Date(getRange(req.Msg.Window))),
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
				Time:     []*timestamppb.Timestamp{timestamppb.New(time.Unix(0, window.Timestamp.Data))},
				Count:    []int64{window.Type.Data},
			}
		} else {
			endpoint.Time = append(endpoint.Time, timestamppb.New(time.Unix(0, window.Timestamp.Data)))
			endpoint.Count = append(endpoint.Count, window.Type.Data)
		}
	}
	return &connect.Response[system.AggregateHitsResponse]{Msg: &system.AggregateHitsResponse{
		Endpoints: endpoints,
	}}, nil
}

// getRange converts the api aggregate window to a lakedb range.
func getRange(window system.AggregateWindow) lake.DateRange {
	switch window {
	case system.AggregateWindow_Minute:
		return lake.DateMinute
	case system.AggregateWindow_Hour:
		return lake.DateHour
	case system.AggregateWindow_Day:
		return lake.DateDay
	case system.AggregateWindow_Month:
		return lake.DateMonth
	}
	return lake.DateHour
}

// getLevel parses the raw slog level and defaults to slog.Info in case of a malformed input.
func getLevel(rawLevel string) (level slog.Level) {
	level.UnmarshalText([]byte(rawLevel))
	return level
}
