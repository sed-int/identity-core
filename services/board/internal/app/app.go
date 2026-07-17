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
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/go-sql-driver/mysql"

	boardv1 "identity-service/api/gen/board/v1"
	"identity-service/pkg/jwks"
	"identity-service/services/board/internal/config"
	deliverygrpc "identity-service/services/board/internal/delivery/grpc"
	mysqlrepo "identity-service/services/board/internal/repo/mysql"
	"identity-service/services/board/internal/usecase"
)

type App struct {
	cfg        config.Config
	log        zerolog.Logger
	db         *sql.DB
	grpcServer *grpc.Server
	httpServer *http.Server
}

func New(ctx context.Context, cfg config.Config, log zerolog.Logger) (*App, error) {
	db, err := openMySQL(ctx, cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}

	// The ONLY link to the identity service: its public keys, over HTTP.
	verifier := jwks.New(cfg.JWKSURL, cfg.Issuer, cfg.Audience)
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
