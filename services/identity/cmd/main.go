package main

import "identity-service/pkg/logger"

func main() {
	log := logger.New("identity")
	log.Info().Msg("identity service placeholder — implemented in Phase 2")
}
