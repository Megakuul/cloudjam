// auth provides a connectrpc middleware (interceptor) that handles authn and authz.
package auth

import (
	"context"
	"strings"

	"codeberg.org/megakuul/cloudjam/internal/rbac"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"connectrpc.com/connect"
)

type contextKey string

var claimsKey = contextKey("claims")

// Claims extracts the user claims form the ctx injected by the auth Interceptor.
// This function will panic if the request had no auth interceptor that injects the claims...
func Claims(ctx context.Context) *token.TokenClaims {
	return ctx.Value(claimsKey).(*token.TokenClaims)
}

type Interceptor struct {
	authorizer *rbac.Authorizer
	issuer     *token.Issuer
}

func New(issuer *token.Issuer, authorizer *rbac.Authorizer) *Interceptor {
	return &Interceptor{
		authorizer: authorizer,
		issuer:     issuer,
	}
}

// authenticate authenticates and authorizes the request.
// returns a context with auth information and a deadline that ends as soon as the token or the rbac cache expires.
func (v *Interceptor) authenticate(ctx context.Context, spec connect.Spec, authHeader string) (context.Context, context.CancelFunc, error) {
	token := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := v.issuer.Verify(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	deadline, err := v.authorizer.Check(ctx, claims.Subject, spec.Procedure)
	if err != nil {
		return nil, nil, err
	}
	if deadline.After(claims.ExpiresAt.Time) {
		deadline = claims.ExpiresAt.Time
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	return context.WithValue(ctx, claimsKey, claims), cancel, nil
}

func (v *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, cancel, err := v.authenticate(ctx, req.Spec(), req.Header().Get("Authorization"))
		if err != nil {
			return nil, err
		}
		defer cancel()
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
		ctx, cancel, err := v.authenticate(ctx, conn.Spec(), conn.RequestHeader().Get("Authorization"))
		if err != nil {
			return err
		}
		defer cancel()
		return next(ctx, conn)
	}
}
