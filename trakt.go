package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	authorizationHeaderName      = "Authorization"
	contentTypeHeaderName        = "Content-Type"
	traktAccessTokenKey          = "TRAKT_ACCESS_TOKEN"
	traktApiKeyHeaderName        = "trakt-api-key"
	traktApiVersionHeaderName    = "trakt-api-version"
	traktBasePath                = "https://api.trakt.tv/"
	traktClientIdKey             = "TRAKT_CLIENT_ID"
	traktUserIdKey               = "TRAKT_USER_ID"
	traktWatchlistPath           = "sync/watchlist/"
	traktWatchlistRemovePath     = "sync/watchlist/remove/"
	traktUserListItemsPath       = "users/%s/lists/%s/items/"
	traktUserListItemsRemovePath = "users/%s/lists/%s/items/remove/"
	traktUserListPath            = "users/%s/lists/"
	traktRatingsPath             = "sync/ratings/"
	traktRatingsRemovePath       = "sync/ratings/remove/"
)

type traktConfig struct {
	accessToken  string
	clientId     string
	clientSecret string
	traktUserId  string
}

type traktClient struct {
	endpoint         string
	client           *http.Client
	config           traktConfig
	retryMaxAttempts int
}

type requestParams struct {
	method string
	path   string
	body   interface{}
}

type Ids struct {
	Imdb string `json:"imdb"`
}

type traktItemSpec struct {
	Rating  int    `json:"rating,omitempty"`
	RatedAt string `json:"rated_at,omitempty"`
	Ids     Ids    `json:"ids"`
}

type traktItem struct {
	Type    string        `json:"type"`
	Movie   traktItemSpec `json:"movie,omitempty"`
	Show    traktItemSpec `json:"show,omitempty"`
	Episode traktItemSpec `json:"episode,omitempty"`
}

type traktListBody struct {
	Movies   []traktItemSpec `json:"movies,omitempty"`
	Shows    []traktItemSpec `json:"shows,omitempty"`
	Episodes []traktItemSpec `json:"episodes,omitempty"`
}

type traktListAddBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Privacy        string `json:"privacy"`
	DisplayNumbers bool   `json:"display_numbers"`
	AllowComments  bool   `json:"allow_comments"`
	SortBy         string `json:"sort_by"`
	SortHow        string `json:"sort_how"`
}

type traktCrudItem struct {
	Movies   int `json:"movies,omitempty"`
	Shows    int `json:"shows,omitempty"`
	Episodes int `json:"episodes,omitempty"`
}

type traktResponse struct {
	Item     string        `json:"item,omitempty"`
	Added    traktCrudItem `json:"added,omitempty"`
	Deleted  traktCrudItem `json:"deleted,omitempty"`
	Existing traktCrudItem `json:"existing,omitempty"`
	NotFound traktListBody `json:"not_found,omitempty"`
}

func newTraktClient() *traktClient {
	return &traktClient{
		endpoint: traktBasePath,
		client: &http.Client{
			Timeout: time.Second * 15,
			Transport: &http.Transport{
				IdleConnTimeout: time.Second * 5,
			},
		},
		config: traktConfig{
			accessToken: os.Getenv(traktAccessTokenKey),
			clientId:    os.Getenv(traktClientIdKey),
			traktUserId: os.Getenv(traktUserIdKey),
		},
		retryMaxAttempts: 5,
	}
}

func (tc *traktClient) doRequest(params requestParams) (*http.Response, error) {
	retries := 0
	for {
		if retries == tc.retryMaxAttempts {
			return nil, fmt.Errorf("reached max retry attempts")
		}
		req, err := http.NewRequest(params.method, tc.endpoint+params.path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Add(traktApiVersionHeaderName, "2")
		req.Header.Add(contentTypeHeaderName, "application/json")
		req.Header.Add(traktApiKeyHeaderName, tc.config.clientId)
		req.Header.Add(authorizationHeaderName, fmt.Sprintf("Bearer %s", tc.config.accessToken))
		if params.body != nil {
			body, err := json.Marshal(params.body)
			if err != nil {
				return nil, err
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		res, err := tc.client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusTooManyRequests {
			retryAfter, err := strconv.Atoi(res.Header.Get("Retry-After"))
			if err != nil {
				return nil, err
			}
			drainBody(res.Body)
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		}
		return res, nil
	}
}

func (tc *traktClient) watchlistItemsGet() []traktItem {
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktWatchlistPath,
	})
	if err != nil {
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktItems(res.Body)
}

func (tc *traktClient) watchlistItemsAdd(imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistPath,
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *traktClient) watchlistItemsRemove(imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistRemovePath,
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *traktClient) listItemsGet(listId string) ([]traktItem, error) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.config.traktUserId, listId),
	})
	if err != nil {
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, errNotFound
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	return readTraktItems(res.Body), nil
}

func (tc *traktClient) listItemsAdd(listId string, imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.config.traktUserId, listId),
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *traktClient) listItemsRemove(listId string, imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsRemovePath, tc.config.traktUserId, listId),
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *traktClient) listAdd(listId, listName string) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListPath, tc.config.traktUserId),
		body: traktListAddBody{
			Name:           listName,
			Description:    fmt.Sprintf("list created by https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
			Privacy:        "public",
			DisplayNumbers: false,
			AllowComments:  true,
			SortBy:         "rank",
			SortHow:        "asc",
		},
	})
	if err != nil {
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.config.traktUserId)
}

func (tc *traktClient) ratingsGet() []traktItem {
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktRatingsPath,
	})
	if err != nil {
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktItems(res.Body)
}

func (tc *traktClient) ratingsAdd(imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktRatingsPath,
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *traktClient) ratingsRemove(imdbItems []imdbItem) {
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktRatingsRemovePath,
		body:   mapImdbItemsToTraktBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.config.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func mapImdbItemsToTraktBody(imdbItems []imdbItem) traktListBody {
	body := traktListBody{}
	for _, item := range imdbItems {
		listItem := traktItemSpec{
			Ids: Ids{
				Imdb: item.id,
			},
		}
		if item.rating != nil && item.ratingDate != nil {
			listItem.Rating = *item.rating
			listItem.RatedAt = item.ratingDate.UTC().String()
		}
		switch item.titleType {
		case "movie":
			body.Movies = append(body.Movies, listItem)
		case "tvSeries":
			body.Shows = append(body.Shows, listItem)
		case "tvMiniSeries":
			body.Shows = append(body.Shows, listItem)
		case "tvEpisode":
			body.Episodes = append(body.Episodes, listItem)
		default:
			continue
		}
	}
	return body
}

func readTraktItems(body io.ReadCloser) []traktItem {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	var traktListItems []traktItem
	err = json.Unmarshal(data, &traktListItems)
	if err != nil {
		log.Fatalf("error unmarshalling trakt list items: %v", err)
	}
	return traktListItems
}

func readTraktResponse(body io.ReadCloser, item string) {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	res := traktResponse{
		Item: item,
	}
	if err := json.Unmarshal(data, &res); err != nil {
		log.Fatalf("failed unmarshalling trakt response")
	}
	prettyPrint, err := json.MarshalIndent(res, "", "    ")
	if err != nil {
		log.Fatalf("failed marshalling trakt response")
	}
	log.Printf("\n%v", string(prettyPrint))
}
