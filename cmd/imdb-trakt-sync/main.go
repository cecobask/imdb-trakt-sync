package main

import (
	"github.com/cecobask/imdb-trakt-sync/pkg/providers"
	"github.com/cecobask/imdb-trakt-sync/pkg/providers/imdb"
	"github.com/cecobask/imdb-trakt-sync/pkg/providers/trakt"
	_ "github.com/joho/godotenv/autoload"
	"log"
	"os"
)

func main() {
	validateEnvVars()
	providers.Sync()
}

func validateEnvVars() {
	_, ok := os.LookupEnv(imdb.ListIdsKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.ListIdsKey)
	}
	_, ok = os.LookupEnv(imdb.CookieAtMainKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.CookieAtMainKey)
	}
	_, ok = os.LookupEnv(imdb.CookieUbidMainKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.CookieUbidMainKey)
	}
	_, ok = os.LookupEnv(trakt.ClientIdKey)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.ClientIdKey)
	}
	_, ok = os.LookupEnv(trakt.AccessTokenKey)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.AccessTokenKey)
	}
}
