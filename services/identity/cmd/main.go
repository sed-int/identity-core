package main

import (
	"context"
	"database/sql"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/go-sql-driver/mysql"

	identityv1 "identity-service/api/gen/identity/v1"
	"identity-service/pkg/logger"
	"identity-service/services/identity/internal/config"
	deliverygrpc "identity-service/services/identity/internal/delivery/grpc"
	"identity-service/services/identity/internal/otp"
	mysqlrepo "identity-service/services/identity/internal/repo/mysql"
	"identity-service/services/identity/internal/rtr"
	"identity-service/services/identity/internal/token"
	"identity-service/services/identity/internal/usecase"
)

func main() {
	cfg := config.Load()
	log := logger.New("identity")

	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("open mysql")
	}
	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("ping mysql")
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal().Err(err).Msg("ping redis")
	}

	key, err := token.LoadOrGenerateKey(cfg.SigningKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msg("load signing key")
	}
	issuer := token.NewIssuer(key, cfg.Issuer)

	auth := usecase.NewAuth(
		mysqlrepo.NewUserRepo(db),
		otp.NewStore(rdb),
		rtr.NewStore(rdb),
		issuer,
		log,
		cfg.DevMode,
	)

	// gRPC server
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(deliverygrpc.LoggingInterceptor(log)))
	identityv1.RegisterIdentityServiceServer(grpcServer, deliverygrpc.NewHandler(auth))

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("listen grpc")
	}
	go func() {
		log.Info().Str("addr", cfg.GRPCAddr).Msg("gRPC server listening")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("grpc serve")
		}
	}()

	// HTTP: gRPC-gateway (REST proxy) + plain OIDC endpoints (PRD §4.1)
	gwMux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerFromEndpoint(
		context.Background(), gwMux, cfg.GRPCAddr,
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	); err != nil {
		log.Fatal().Err(err).Msg("register gateway")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth2/v1/jwks", token.JWKSHandler(cfg.Issuer, key))
	mux.HandleFunc("GET /.well-known/openid-configuration", token.DiscoveryHandler(cfg.Issuer))
	mux.Handle("/", gwMux)

	log.Info().Str("addr", cfg.HTTPAddr).Bool("dev_mode", cfg.DevMode).Msg("HTTP server listening")
	if err := http.ListenAndServe(cfg.HTTPAddr, mux); err != nil {
		log.Fatal().Err(err).Msg("http serve")
	}
}
