package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	gokitlog "github.com/go-kit/log"

	authmiddleware "codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/bootstrap"
	"codeberg.org/megakuul/cloudjam/internal/olap"
	"codeberg.org/megakuul/cloudjam/internal/olap/log"
	"codeberg.org/megakuul/cloudjam/internal/olap/request"
	"codeberg.org/megakuul/cloudjam/internal/rbac"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/admin/user"
	"codeberg.org/megakuul/cloudjam/internal/server/v1/auth"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user/userconnect"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth/authconnect"
	"codeberg.org/megakuul/cloudjam/web"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/golang-jwt/jwt/v5"
	"github.com/megakuul/dynamitedb"
	"github.com/polarsignals/frostdb"
	"github.com/thanos-io/objstore/providers/s3"
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
	slog.Debug(fmt.Sprintf("initializing dynamitedb bucket at '%s'...", opts.BucketURL))
	bucket, err := dynamitedb.New(ctx, opts.BucketURL, opts.BucketName,
		dynamitedb.WithRegion(opts.BucketRegion),
		dynamitedb.WithCredentials(opts.BucketAccessKey, opts.BucketSecretKey),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize dynamitedb bucket: %v", err)
	}

	olapBucketConfig := s3.Config{
		Bucket:    opts.BucketName,
		Region:    opts.BucketRegion,
		AccessKey: opts.BucketAccessKey,
		SecretKey: opts.BucketSecretKey,
	}
	if bucketEndpoint, ok := strings.CutPrefix(opts.BucketURL, "https://"); ok {
		olapBucketConfig.Endpoint = bucketEndpoint
	} else if bucketEndpoint, ok := strings.CutPrefix(opts.BucketURL, "http://"); ok {
		olapBucketConfig.Endpoint = bucketEndpoint
		olapBucketConfig.Insecure = true
	}
	olapBucket, err := s3.NewBucketWithConfig(gokitlog.NewNopLogger(), olapBucketConfig, "olap")
	if err != nil {
		return fmt.Errorf("failed to initialize olap bucket: %v", err)
	}
	olapStore, err := frostdb.New(
		frostdb.WithReadWriteStorage(frostdb.NewDefaultObjstoreBucket(olapBucket)),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize olap frostdb store: %v", err)
	}
	defer olapStore.Close()
	olapDatabase, err := olapStore.DB(ctx, "olap")
	if err != nil {
		return fmt.Errorf("failed to initialize olap frostdb database: %v", err)
	}
	defer olapDatabase.Close()

	// olapEngine := query.NewEngine(memory.DefaultAllocator, olapDatabase.TableProvider())

	logTable, err := frostdb.NewGenericTable[log.Log](olapDatabase, "log", memory.DefaultAllocator)
	if err != nil {
		return fmt.Errorf("failed to initialize log olap table: %v", err)
	}
	// logController := log.New("log", olapEngine)

	slog.SetDefault(slog.New(slog.NewMultiHandler(
		slog.Default().Handler(),
		log.NewSink(olap.NewLocalInserter(olapDatabase, logTable), &log.SinkOptions{Level: slog.LevelDebug}),
	)))

	requestTable, err := frostdb.NewGenericTable[request.Request](olapDatabase, "request", memory.DefaultAllocator)
	if err != nil {
		return fmt.Errorf("failed to initialize request olap table: %v", err)
	}
	// requestController := request.New("request", olapEngine)
	requestInserter := olap.NewLocalInserter(olapDatabase, requestTable)

	code, err := bootstrap.CreateAdministrator(ctx, opts.AdminEmail, bucket)
	if err != nil {
		return fmt.Errorf("failed to initialize administrator: %v", err)
	} else if code != "" {
		slog.Info(fmt.Sprintf("admin user (%s) registration code: '%s'", opts.AdminEmail, code))
	}

	apiMux := http.NewServeMux()
	authorizer := rbac.New(bucket, opts.PolicyCacheTimeout)
	apiMux.Handle(authconnect.NewAuthServiceHandler(auth.New(slog.With("system", "svc.auth"), bucket, issuer),
		connect.WithInterceptors(
			request.NewInterceptor(requestInserter),
			validate.NewInterceptor(),
		),
	))
	apiMux.Handle(userconnect.NewUserServiceHandler(user.New(slog.With("system", "svc.admin.user"), bucket),
		connect.WithInterceptors(
			request.NewInterceptor(requestInserter),
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
