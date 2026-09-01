// Package app is the board service's composition root — same shape as the
// identity service: main() only does config + logger + App.
package app

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/go-sql-driver/mysql"

	boardv1 "identity-service/api/gen/board/v1"
	"identity-service/pkg/jwks"
	"identity-service/services/board/internal/config"
	"identity-service/services/board/internal/consumer"
	deliverygrpc "identity-service/services/board/internal/delivery/grpc"
	mysqlrepo "identity-service/services/board/internal/repo/mysql"
	"identity-service/services/board/internal/usecase"
)

type App struct {
	cfg        config.Config
	log        zerolog.Logger
	db         *sql.DB
	rdb        *redis.Client
	consumer   *consumer.Consumer
	grpcServer *grpc.Server
	httpServer *http.Server
}

func New(ctx context.Context, cfg config.Config, log zerolog.Logger) (*App, error) {
	db, err := openMySQL(ctx, cfg)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		db.Close()
		return nil, err
	}

	// The ONLY link to the identity service: its public keys over HTTP, and
	// its events over the stream — never its database or its APIs.
	verifier := jwks.New(cfg.JWKSURL, cfg.Issuer, cfg.Audience, jwks.Options{
		CacheTTL:        cfg.JWKSCacheTTL,
		RefreshCooldown: cfg.JWKSRefreshCooldown,
		BreakerTimeout:  cfg.JWKSBreakerTimeout,
		OnStateChange: func(name, from, to string) {
			log.Warn().Str("breaker", name).Str("from", from).Str("to", to).Msg("circuit breaker state changed")
		},
	})
	board := usecase.NewBoard(mysqlrepo.NewPostRepo(db))

	gwMux := runtime.NewServeMux()
	if err := boardv1.RegisterBoardServiceHandlerFromEndpoint(
		ctx, gwMux, cfg.GRPCAddr,
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	); err != nil {
		return nil, err
	}

	return &App{
		cfg:        cfg,
		log:        log,
		db:         db,
		rdb:        rdb,
		consumer:   consumer.New(rdb, db, log),
		grpcServer: deliverygrpc.NewServer(log, board, verifier),
		httpServer: &http.Server{Addr: cfg.HTTPAddr, Handler: gwMux},
	}, nil
}

// Run serves gRPC and HTTP until ctx is cancelled or a server fails.
func (a *App) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", a.cfg.GRPCAddr)
	if err != nil {
		return err
	}

	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	go a.consumer.Run(consumerCtx)

	errCh := make(chan error, 2)
	go func() {
		a.log.Info().Str("addr", a.cfg.GRPCAddr).Msg("gRPC server listening")
		errCh <- a.grpcServer.Serve(lis)
	}()
	go func() {
		a.log.Info().Str("addr", a.cfg.HTTPAddr).Msg("HTTP server listening")
		if err := a.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		a.log.Info().Msg("shutting down")
		err = nil
	case err = <-errCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.httpServer.Shutdown(shutdownCtx)
	a.grpcServer.GracefulStop()
	a.rdb.Close()
	a.db.Close()
	return err
}

func openMySQL(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
