// Package app is the identity service's composition root: it opens
// infrastructure connections, wires the layers together, and runs the servers.
// main() should do nothing beyond config + logger + App.
package app

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	_ "github.com/go-sql-driver/mysql"

	"identity-service/services/identity/internal/config"
	deliverygrpc "identity-service/services/identity/internal/delivery/grpc"
	deliveryhttp "identity-service/services/identity/internal/delivery/http"
	"identity-service/services/identity/internal/otp"
	"identity-service/services/identity/internal/outbox"
	mysqlrepo "identity-service/services/identity/internal/repo/mysql"
	"identity-service/services/identity/internal/rtr"
	"identity-service/services/identity/internal/token"
	"identity-service/services/identity/internal/usecase"
)

type App struct {
	cfg        config.Config
	log        zerolog.Logger
	db         *sql.DB
	rdb        *redis.Client
	relay      *outbox.Relay
	grpcServer *grpc.Server
	httpServer *http.Server
}

func New(ctx context.Context, cfg config.Config, log zerolog.Logger) (*App, error) {
	db, err := openMySQL(ctx, cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}
	rdb, err := openRedis(ctx, cfg.RedisAddr)
	if err != nil {
		return nil, err
	}
	key, err := token.LoadOrGenerateKey(cfg.SigningKeyPath)
	if err != nil {
		return nil, err
	}

	auth := usecase.NewAuth(
		mysqlrepo.NewUserRepo(db),
		otp.NewStore(rdb),
		rtr.NewStore(rdb),
		token.NewIssuer(key, cfg.Issuer),
		log,
		cfg.DevMode,
	)

	httpHandler, err := deliveryhttp.NewHandler(ctx, cfg.Issuer, cfg.GRPCAddr, key)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:        cfg,
		log:        log,
		db:         db,
		rdb:        rdb,
		relay:      outbox.NewRelay(db, rdb, log),
		grpcServer: deliverygrpc.NewServer(log, auth),
		httpServer: &http.Server{Addr: cfg.HTTPAddr, Handler: httpHandler},
	}, nil
}

// Run serves gRPC and HTTP until ctx is cancelled or a server fails,
// then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", a.cfg.GRPCAddr)
	if err != nil {
		return err
	}

	relayCtx, stopRelay := context.WithCancel(context.Background())
	defer stopRelay()
	go a.relay.Run(relayCtx)

	errCh := make(chan error, 2)
	go func() {
		a.log.Info().Str("addr", a.cfg.GRPCAddr).Msg("gRPC server listening")
		errCh <- a.grpcServer.Serve(lis)
	}()
	go func() {
		a.log.Info().Str("addr", a.cfg.HTTPAddr).Bool("dev_mode", a.cfg.DevMode).Msg("HTTP server listening")
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

func openMySQL(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openRedis(ctx context.Context, addr string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, err
	}
	return rdb, nil
}
