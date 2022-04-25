package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	authorizationHeaderName   = "Authorization"
	contentTypeHeaderName     = "Content-Type"
	traktAccessTokenKey       = "TRAKT_ACCESS_TOKEN"
	traktApiKeyHeaderName     = "trakt-api-key"
	traktApiVersionHeaderName = "trakt-api-version"
	traktBasePath             = "https://api.trakt.tv/"
	traktClientIdKey          = "TRAKT_CLIENT_ID"
	traktClientSecretKey      = "TRAKT_CLIENT_SECRET"
	traktUserIdKey            = "TRAKT_USER_ID"
	traktWatchlistPath        = "sync/watchlist/"
)

type traktClient struct {
	endpoint    string
	client      *http.Client
	credentials traktCredentials
}

type traktCredentials struct {
	clientId     string
	clientSecret string
	accessToken  string
}

type requestParams struct {
	method string
	path   string
	body   interface{}
}

type traktListItem struct {
	Type  string `json:"type"`
	Movie struct {
		Ids struct {
			Imdb string `json:"imdb"`
		} `json:"ids"`
	} `json:"movie,omitempty"`
	Show struct {
		Ids struct {
			Imdb string `json:"imdb"`
		} `json:"ids"`
	} `json:"show,omitempty"`
}

func newTraktClient() *traktClient {
	return &traktClient{
		endpoint: traktBasePath,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
			},
		},
		credentials: traktCredentials{
			clientId:     os.Getenv(traktClientIdKey),
			clientSecret: os.Getenv(traktClientSecretKey),
			accessToken:  os.Getenv(traktAccessTokenKey),
		},
	}
}

func (tc *traktClient) doRequest(params requestParams) (*http.Response, error) {
	req, err := http.NewRequest(params.method, tc.endpoint+params.path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(traktApiVersionHeaderName, "2")
	req.Header.Add(contentTypeHeaderName, "application/json")
	req.Header.Add(traktApiKeyHeaderName, tc.credentials.clientId)
	req.Header.Add(authorizationHeaderName, fmt.Sprintf("Bearer %s", tc.credentials.accessToken))
	if params.body != nil {
		body, err := json.Marshal(params.body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	resp, err := tc.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (tc *traktClient) getWatchlist() []traktListItem {
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktWatchlistPath,
	})
	if err != nil {
		log.Fatalf("error retrieving trakt watchlist: %v", err)
	}
	defer closeBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusUnauthorized:
		log.Fatalf("error retrieving trakt watchlist: %v", res.StatusCode)
	default:
		log.Fatalf("error retrieving trakt watchlist: %v", res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func readTraktListItems(body io.ReadCloser) []traktListItem {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	var traktListItems []traktListItem
	err = json.Unmarshal(data, &traktListItems)
	if err != nil {
		log.Fatalf("error unmarshalling trakt list: %v", err)
	}
	return traktListItems
}
