package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/PeladoCollado/imager/orchestrator/app"
	"github.com/PeladoCollado/imager/orchestrator/logger"
)

func main() {
	cfg, err := app.ParseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse configuration: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, cfg, app.RunOptions{}); err != nil {
		logger.Logger.Error("Unable to run orchestrator", err)
		os.Exit(1)
	}
}
