//go:build !windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runAgent(cfg agentConfig, logger *log.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runAgentLoop(ctx, cfg, logger)
}
