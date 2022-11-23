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
	_, ok := os.LookupEnv(imdb.EnvVarKeyListIds)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.EnvVarKeyListIds)
	}
	_, ok = os.LookupEnv(imdb.EnvVarKeyCookieAtMain)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.EnvVarKeyCookieAtMain)
	}
	_, ok = os.LookupEnv(imdb.EnvVarKeyCookieUbidMain)
	if !ok {
		log.Fatalf("environment variable %s is required", imdb.EnvVarKeyCookieUbidMain)
	}
	_, ok = os.LookupEnv(trakt.EnvVarKeyClientId)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.EnvVarKeyClientId)
	}
	_, ok = os.LookupEnv(trakt.EnvVarKeyClientSecret)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.EnvVarKeyClientId)
	}
	_, ok = os.LookupEnv(trakt.EnvVarKeyUsername)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.EnvVarKeyUsername)
	}
	_, ok = os.LookupEnv(trakt.EnvVarKeyPassword)
	if !ok {
		log.Fatalf("environment variable %s is required", trakt.EnvVarKeyPassword)
	}
}
