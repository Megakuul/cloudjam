package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	authmiddleware "codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/bootstrap"
	"codeberg.org/megakuul/cloudjam/internal/olap"
	"codeberg.org/megakuul/cloudjam/internal/olap/middleware"
	"codeberg.org/megakuul/cloudjam/internal/rbac"
	rbacsvc "codeberg.org/megakuul/cloudjam/internal/server/v1/admin/rbac"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/role"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/system"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/rbac/rbacconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/role/roleconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system/systemconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/web"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/megakuul/dynamitedb"
	lake "github.com/megakuul/lake"
	"golang.org/x/sync/errgroup"
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
	slog.Debug(fmt.Sprintf("initializing oltp bucket at '%s'...", opts.OLTPBucketURL))
	oltpBucket, err := dynamitedb.New(ctx, opts.OLTPBucketURL, opts.OLTPBucketName,
		dynamitedb.WithRegion(opts.OLTPBucketRegion),
		dynamitedb.WithCredentials(opts.OLTPBucketAccessKey, opts.OLTPBucketSecretKey),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize dynamitedb oltp bucket: %v", err)
	}

	slog.Debug(fmt.Sprintf("initializing olap bucket at '%s'...", opts.OLAPBucketURL))
	olapBucket, err := lake.New(ctx, opts.OLAPBucketURL, opts.OLAPBucketName,
		lake.WithRegion(opts.OLAPBucketRegion),
		lake.WithCredentials(opts.OLAPBucketAccessKey, opts.OLAPBucketSecretKey),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize lakedb olap bucket: %v", err)
	}

	logIngestor := lake.NewIngestor(olapBucket, lake.WithAutoCommit[olap.Log](time.Second*10))
	defer logIngestor.Close(ctx)

	slog.Debug("initializing logging backend...")
	slog.SetDefault(slog.New(slog.NewMultiHandler(
		slog.Default().Handler(),
		middleware.NewLogSink(logIngestor, &middleware.LogSinkOptions{Level: slog.LevelDebug}),
	)))

	requestIngestor := lake.NewIngestor(olapBucket, lake.WithAutoCommit[olap.Request](time.Second*10))
	defer requestIngestor.Close(ctx)

	code, err := bootstrap.CreateAdministrator(ctx, opts.AdminEmail, oltpBucket)
	if err != nil {
		return fmt.Errorf("failed to initialize administrator: %v", err)
	} else if code != "" {
		slog.Info(fmt.Sprintf("admin user (%s) registration code: '%s'", opts.AdminEmail, code))
	}

	apiMux := http.NewServeMux()
	authorizer := rbac.New(oltpBucket, opts.PolicyCacheTimeout)
	apiMux.Handle(authconnect.NewAuthServiceHandler(auth.New(slog.With("system", "svc.auth"), oltpBucket, issuer),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(userconnect.NewUserServiceHandler(user.New(slog.With("system", "svc.admin.user"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(roleconnect.NewRoleServiceHandler(role.New(slog.With("system", "svc.admin.role"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(rbacconnect.NewRBACServiceHandler(rbacsvc.New(slog.With("system", "svc.admin.rbac"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(systemconnect.NewSystemServiceHandler(system.New(slog.With("system", "svc.admin.system"), oltpBucket, olapBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	mux.Handle("/api/", http.StripPrefix("/api", apiMux))

	server := http.Server{
		Addr:     opts.Addr,
		Handler:  mux,
		ErrorLog: slog.NewLogLogger(slog.With("system", "http.server").Handler(), slog.LevelWarn),
	}

	runGroups, runCtx := errgroup.WithContext(ctx)

	requestCompactor := lake.NewCompactor[olap.Request](olapBucket)
	logCompactor := lake.NewCompactor[olap.Log](olapBucket)
	runGroups.Go(func() error {
		for {
			slog.Info("starting olap compaction process...")
			if err := logCompactor.Compact(runCtx); err != nil {
				slog.Error(fmt.Sprintf("log compaction failure: %v", err))
			}
			if err := requestCompactor.Compact(runCtx); err != nil {
				slog.Error(fmt.Sprintf("request compaction failure: %v", err))
			}
			select {
			case <-time.After(time.Hour):
				continue
			case <-runCtx.Done():
				return nil
			}
		}
	})
	runGroups.Go(func() error {
		<-runCtx.Done()
		return server.Close()
	})
	runGroups.Go(func() error {
		slog.Info(fmt.Sprintf("starting hornet server at http://%s", opts.Addr))
		return server.ListenAndServe()
	})
	if err = runGroups.Wait(); err != nil && !errors.Is(http.ErrServerClosed, err) {
		return err
	}
	return nil
}
