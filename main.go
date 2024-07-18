package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cecobask/imdb-trakt-sync/cmd/root"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	defer stop()
	err := root.NewCommand(ctx).Execute()
	if err != nil {
		os.Exit(1)
	}
}
