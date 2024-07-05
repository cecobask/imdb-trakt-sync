package main

import (
	"context"
	"os"

	"github.com/cecobask/imdb-trakt-sync/cmd/root"
)

func main() {
	ctx := context.Background()
	err := root.NewCommand(ctx).Execute()
	if err != nil {
		os.Exit(1)
	}
}
