package request

import (
	"context"
	"math"
	"net/netip"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap"
	"connectrpc.com/connect"
)

// Interceptor implements a connectrpc middleware that sends a metric about every request to the olap inserter.
type Interceptor struct {
	inserter olap.Inserter[Request]
}

func NewInterceptor(inserter olap.Inserter[Request]) *Interceptor {
	return &Interceptor{
		inserter: inserter,
	}
}

// emitMetric inserts an entry to the olap system that contains anonymized information about the request.
func (v *Interceptor) emitMetric(ctx context.Context, stream bool, peer, userAgent, procedure string) error {
	peerIP, err := netip.ParseAddr(peer)
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
		UserAgent: userAgent[0:int(math.Min(float64(len(userAgent)), 50.0))],
		Stream:    stream,
		Source:    anonymPeerIP,
	}); err != nil {
		return err
	}
	return nil
}

func (v *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := v.emitMetric(ctx, true,
			req.Peer().Addr,
			req.Spec().Procedure,
			req.Header().Get("User-Agent"),
		); err != nil {
			return nil, err
		}
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
		if err := v.emitMetric(ctx, true,
			conn.Peer().Addr,
			conn.Spec().Procedure,
			conn.RequestHeader().Get("User-Agent"),
		); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
