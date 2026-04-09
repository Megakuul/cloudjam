package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	authmiddleware "codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/bootstrap"
	"codeberg.org/megakuul/cloudjam/internal/rbac"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/web"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/golang-jwt/jwt/v5"

	"gocloud.dev/docstore/mongodocstore"
	_ "gocloud.dev/docstore/mongodocstore"
)

func Start(ctx context.Context, opts *Options) error {
	mux := http.NewServeMux()
	if opts.Dev {
		slog.Warn("hornet runs in development mode. Don't use this in production! 🐝")
		url, err := url.Parse(fmt.Sprint("http://", opts.DevWebAddr))
		if err != nil {
			return err
		}
		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.ErrorLog = slog.NewLogLogger(slog.With("system", "dev.proxy").Handler(), slog.LevelWarn)
		mux.Handle("/", proxy)
	} else {
		mux.Handle("/", http.FileServerFS(web.Files))
	}
	issuer := token.New(opts.TokenIssuer, opts.TokenLifetime, jwt.SigningMethodHS256, []byte(opts.TokenSecret), func(ctx context.Context) any {
		return []byte(opts.TokenSecret)
	})
	client, err := mongodocstore.Dial(ctx, opts.DatabaseSource)
	if err != nil {
		return err
	}
	coll, err := mongodocstore.OpenCollection(client.Database(opts.DatabaseName).Collection(opts.DatabaseCollection), "pk", nil)
	if err != nil {
		return err
	}
	defer coll.Close()

	code, err := bootstrap.CreateAdministrator(ctx, opts.AdminEmail, coll)
	if err != nil {
		return fmt.Errorf("failed to initialize administrator: %v", err)
	} else if code != "" {
		slog.Info(fmt.Sprintf("admin user registration code: '%s'", code))
	}

	authorizer := rbac.New(coll, opts.PolicyCacheTimeout)
	mux.Handle(authconnect.NewAuthServiceHandler(auth.New(slog.With("system", "svc.auth"), coll, issuer),
		connect.WithInterceptors(validate.NewInterceptor()),
	))
	mux.Handle(userconnect.NewUserServiceHandler(user.New(slog.With("system", "svc.admin.user"), coll),
		connect.WithInterceptors(
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))

	server := http.Server{
		Addr:     opts.Addr,
		Handler:  mux,
		ErrorLog: slog.NewLogLogger(slog.With("system", "http.server").Handler(), slog.LevelWarn),
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()
	slog.Info(fmt.Sprintf("starting hornet server at http://%s", opts.Addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(http.ErrServerClosed, err) {
		return err
	}
	return nil
}
