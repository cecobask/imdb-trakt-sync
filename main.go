package main

import (
	"os"

	"github.com/cecobask/imdb-trakt-sync/cmd/root"
)

func main() {
	err := root.NewCommand().Execute()
	if err != nil {
		os.Exit(1)
	}
}
