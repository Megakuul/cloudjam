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
	challengerunner "codeberg.org/megakuul/cloudjam/internal/challenge"
	"codeberg.org/megakuul/cloudjam/internal/olap"
	"codeberg.org/megakuul/cloudjam/internal/olap/middleware"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/internal/rbac"
	"codeberg.org/megakuul/cloudjam/internal/scheduler"
	rbacsvc "codeberg.org/megakuul/cloudjam/internal/server/v1/admin/rbac"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/role"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/system"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/cloud/account"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/cloud/definition"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/cloud/provider"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/play/challenge"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/play/game"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/play/team"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/rbac/rbacconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/role/roleconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system/systemconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/account/accountconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/definition/definitionconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/provider/providerconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/challenge/challengeconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/game/gameconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/team/teamconnect"
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
		defer cancel()
		slog.Debug("shutting down log ingestor...")
		if err = logIngestor.Close(ctx); err != nil {
			slog.Error(fmt.Sprintf("failed to close log ingestor: %v", err))
		}
	}()
	requestIngestor := lake.NewIngestor(olapBucket, lake.WithAutoCommit[olap.Request](time.Second*10))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
		defer cancel()
		slog.Debug("shutting down request ingestor...")
		if err = requestIngestor.Close(ctx); err != nil {
			slog.Error(fmt.Sprintf("failed to close request ingestor: %v", err))
		}
	}()

	slog.Debug("initializing logging backend...")
	slog.SetDefault(slog.New(slog.NewMultiHandler(
		slog.Default().Handler(),
		middleware.NewLogSink(logIngestor, &middleware.LogSinkOptions{Level: slog.LevelDebug}),
	)))

	code, err := bootstrap.CreateAdministrator(ctx, opts.AdminEmail, oltpBucket)
	if err != nil {
		return fmt.Errorf("failed to initialize administrator: %v", err)
	} else if code != "" {
		slog.Info(fmt.Sprintf("admin user (%s) registration code: '%s'", opts.AdminEmail, code))
	}

	providerCache := cache.New(oltpBucket)
	pluginCache := challengerunner.NewCache(time.Hour * 24)
	defer pluginCache.Close(ctx)

	runGroups, runCtx := errgroup.WithContext(ctx)

	scheduler := scheduler.New(runCtx)
	runGroups.Go(func() error {
		scheduler.Wait()
		return nil
	})

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
	apiMux.Handle(providerconnect.NewProviderServiceHandler(provider.New(slog.With("system", "svc.cloud.provider"), scheduler, providerCache, oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(accountconnect.NewAccountServiceHandler(account.New(slog.With("system", "svc.cloud.account"), scheduler, providerCache, oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(definitionconnect.NewDefinitionServiceHandler(definition.New(slog.With("system", "svc.cloud.definition"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(gameconnect.NewGameServiceHandler(game.New(slog.With("system", "svc.cloud.game"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(teamconnect.NewTeamServiceHandler(team.New(slog.With("system", "svc.cloud.team"), oltpBucket),
		connect.WithInterceptors(
			middleware.NewRequestTracer(slog.With("system", "olap.request"), requestIngestor),
			authmiddleware.New(issuer, authorizer),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(challengeconnect.NewChallengeServiceHandler(challenge.New(slog.With("system", "svc.cloud.challenge"),
		scheduler, providerCache, pluginCache, oltpBucket, olapBucket,
	),
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
