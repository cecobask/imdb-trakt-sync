package main

import (
	"github.com/cecobask/imdb-trakt-sync/pkg/syncer"
)

func main() {
	syncer.NewSyncer().Run()
}
