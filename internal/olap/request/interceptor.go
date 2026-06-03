package request

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap"
	"connectrpc.com/connect"
)

// Interceptor implements a connectrpc middleware that sends a metric about every request to the olap inserter.
type Interceptor struct {
	logger   *slog.Logger
	inserter olap.Inserter[Request]
}

func NewInterceptor(logger *slog.Logger, inserter olap.Inserter[Request]) *Interceptor {
	return &Interceptor{
		logger:   logger,
		inserter: inserter,
	}
}

// emitMetric inserts an entry to the olap system that contains anonymized information about the request.
func (v *Interceptor) emitMetric(ctx context.Context, stream bool, latency time.Duration, peer, procedure string) error {
	// trim off port number
	segments := strings.Split(peer, ":")
	if len(segments) < 1 {
		return fmt.Errorf("malformed peer address: '%s'", peer)
	}
	peerIP, err := netip.ParseAddr(strings.Trim(strings.Join(segments[:len(segments)-1], ":"), "[]"))
	if err != nil {
		return err
	}
	anonymPeerIP := ""
	if peerIP.Is4() {
		raw := peerIP.As4()
		raw[3] = 0
		anonymPeerIP = netip.AddrFrom4(raw).String()
	}
	if peerIP.Is6() {
		raw := peerIP.As16()
		for i := 4; i < 16; i++ {
			raw[i] = 0
		}
		anonymPeerIP = netip.AddrFrom16(raw).String()
	}
	if peerIP.Is4() {
		rawIp := peerIP.As4()
		rawIp[3] = 0
		anonymPeerIP = string(rawIp[:])
	}
	if err := v.inserter.Insert(ctx, Request{
		Timestamp: uint64(time.Now().Unix()),
		Endpoint:  procedure,
		Latency:   int64(latency),
		Stream:    stream,
		Source:    anonymPeerIP,
	}); err != nil {
		return err
	}
	return nil
}

func (v *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		start := time.Now()
		defer func() {
			if err := v.emitMetric(ctx, false, time.Since(start),
				req.Peer().Addr,
				req.Spec().Procedure,
			); err != nil {
				v.logger.Warn(fmt.Sprintf("failed to emit request metric: %v", err))
			}
		}()
		return next(ctx, req)
	}
}

func (v *Interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

func (v *Interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		start := time.Now()
		defer func() {
			if err := v.emitMetric(ctx, true, time.Since(start),
				conn.Peer().Addr,
				conn.Spec().Procedure,
			); err != nil {
				v.logger.Warn(fmt.Sprintf("failed to emit request metric: %v", err))
			}
		}()
		return next(ctx, conn)
	}
}
