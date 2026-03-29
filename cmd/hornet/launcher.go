package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/web"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/golang-jwt/jwt/v5"
	"gocloud.dev/docstore"
)

func Start(ctx context.Context, opts *Options) error {
	mux := http.NewServeMux()
	if opts.Dev {
		slog.Warn("hornet runs in development mode. Don't use this in production! 🐝")
		url, err := url.Parse(fmt.Sprint("http://", opts.DevWebAddr))
		if err != nil {
			return err
		}
		mux.Handle("/", httputil.NewSingleHostReverseProxy(url))
	} else {
		mux.Handle("GET /", http.FileServerFS(web.Files))
	}
	issuer := token.New(opts.TokenIssuer, jwt.SigningMethodHS256, []byte(opts.TokenSecret), func(ctx context.Context) any {
		return []byte(opts.TokenSecret)
	})
	coll, err := docstore.OpenCollection(ctx, opts.Database)
	if err != nil {
		return err
	}
	mux.Handle(authconnect.NewAuthServiceHandler(auth.New(coll, issuer),
		connect.WithInterceptors(validate.NewInterceptor()),
	))
	mux.Handle(userconnect.NewUserServiceHandler(user.New(coll),
		connect.WithInterceptors(
			validate.NewInterceptor(),
		),
	))

	return nil
}
