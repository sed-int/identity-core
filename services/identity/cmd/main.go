package main

import (
	"context"
	"os/signal"
	"syscall"

	"identity-service/pkg/logger"
	"identity-service/services/identity/internal/app"
	"identity-service/services/identity/internal/config"
)

func main() {
	cfg := config.Load()
	log := logger.New("identity")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("bootstrap")
	}
	if err := a.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("run")
	}
}
