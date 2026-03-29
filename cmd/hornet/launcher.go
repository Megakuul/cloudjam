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
	"codeberg.org/megakuul/cloudjam/internal/rbac"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/web"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/FerretDB/FerretDB/ferretdb"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"golang.org/x/sync/errgroup"

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
	issuer := token.New(opts.TokenIssuer, jwt.SigningMethodHS256, []byte(opts.TokenSecret), func(ctx context.Context) any {
		return []byte(opts.TokenSecret)
	})
	fclient, err := ferretdb.New(&ferretdb.Config{
		Listener: ferretdb.ListenerConfig{
			Unix: opts.DatabaseMongoSocket,
		},
		Logger:        slog.With("system", "ferretdb"),
		Handler:       "postgresql",
		PostgreSQLURL: opts.DatabaseSource,
	})
	if err != nil {
		return err
	}
	mclient, err := mongodocstore.Dial(ctx, fclient.MongoDBURI())
	if err != nil {
		return err
	}
	coll, err := mongodocstore.OpenCollection(mclient.Database(opts.DatabaseMongoName).Collection(opts.DatabaseMongoCollection), "pk", nil)
	if err != nil {
		return err
	}
	defer coll.Close()
	authorizer := rbac.New(coll, opts.PolicyCacheTimeout)
	mux.Handle(authconnect.NewAuthServiceHandler(auth.New(coll, issuer),
		connect.WithInterceptors(validate.NewInterceptor()),
	))
	mux.Handle(userconnect.NewUserServiceHandler(user.New(coll),
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

	errGroup, errCtx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return fclient.Run(errCtx)
	})
	errGroup.Go(func() error {
		defer slog.Info("test")
		return mclient.Ping(errCtx, readpref.Nearest())
	})
	errGroup.Go(func() error {
		go func() {
			<-errCtx.Done()
			server.Close()
		}()
		if err := server.ListenAndServe(); err != nil && !errors.Is(http.ErrServerClosed, err) {
			return err
		}
		return nil
	})
	slog.Info(fmt.Sprintf("starting hornet server at http://%s", opts.Addr))
	return errGroup.Wait()
}
