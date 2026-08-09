package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := app.NewCmd()
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
