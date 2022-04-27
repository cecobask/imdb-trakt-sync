package main

import (
	"errors"
	"io"
	"log"
	"os"
)

var errNotFound = errors.New("resource not found")

func validateEnvVars() {
	_, ok := os.LookupEnv(imdbUserIdKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdbUserIdKey)
	}
	_, ok = os.LookupEnv(imdbWatchlistIdKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdbWatchlistIdKey)
	}
	_, ok = os.LookupEnv(imdbCustomListIdsKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdbCustomListIdsKey)
	}
	_, ok = os.LookupEnv(imdbCookieAtMainKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdbCookieAtMainKey)
	}
	_, ok = os.LookupEnv(imdbCookieUbidMainKey)
	if !ok {
		log.Fatalf("environment variable %s is required", imdbCookieUbidMainKey)
	}
	_, ok = os.LookupEnv(traktUserIdKey)
	if !ok {
		log.Fatalf("environment variable %s is required", traktUserIdKey)
	}
	_, ok = os.LookupEnv(traktClientIdKey)
	if !ok {
		log.Fatalf("environment variable %s is required", traktClientIdKey)
	}
	_, ok = os.LookupEnv(traktAccessTokenKey)
	if !ok {
		log.Fatalf("environment variable %s is required", traktAccessTokenKey)
	}
	_, ok = os.LookupEnv(traktClientSecretKey)
	if !ok {
		log.Fatalf("environment variable %s is required", traktClientSecretKey)
	}
}

func drainBody(body io.ReadCloser) {
	err := body.Close()
	if err != nil {
		log.Fatalf("error closing response body: %v", err)
	}
}
