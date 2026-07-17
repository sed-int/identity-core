package main

import "identity-service/pkg/logger"

func main() {
	log := logger.New("board")
	log.Info().Msg("board service placeholder — implemented in Phase 3")
}
