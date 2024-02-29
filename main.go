package main

import (
	"os"

	"github.com/cecobask/imdb-trakt-sync/cmd"
)

func main() {
	err := cmd.NewRootCommand().Execute()
	if err != nil {
		os.Exit(1)
	}
}
