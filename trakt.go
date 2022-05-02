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
	traktWatchlistPath           = "sync/watchlist/"
	traktWatchlistRemovePath     = "sync/watchlist/remove/"
	traktUserListItemsPath       = "users/%s/lists/%s/items/"
	traktUserListItemsRemovePath = "users/%s/lists/%s/items/remove/"
	traktUserListPath            = "users/%s/lists/%s"
	traktRatingsPath             = "sync/ratings/"
	traktRatingsRemovePath       = "sync/ratings/remove/"
	traktProfilePath             = "users/me/"
	traktHistoryGetPath          = "sync/history/%s/%s?limit=%s"
	traktHistoryPath             = "sync/history/"
	traktHistoryRemovePath       = "sync/history/remove"
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
	Slug string `json:"slug,omitempty"`
}

type traktItemSpec struct {
	Ids     Ids    `json:"ids"`
	RatedAt string `json:"rated_at,omitempty"`
	Rating  int    `json:"rating,omitempty"`
}

type traktItem struct {
	Type      string        `json:"type"`
	RatedAt   string        `json:"rated_at,omitempty"`
	Rating    int           `json:"rating,omitempty"`
	WatchedAt string        `json:"watched_at,omitempty"`
	Movie     traktItemSpec `json:"movie,omitempty"`
	Show      traktItemSpec `json:"show,omitempty"`
	Episode   traktItemSpec `json:"episode,omitempty"`
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

type traktList struct {
	Name string `json:"name"`
	Ids  Ids
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
		},
		retryMaxAttempts: 5,
	}
}

func (tc *traktClient) doRequest(params requestParams) *http.Response {
	retries := 0
	for {
		if retries == tc.retryMaxAttempts {
			log.Fatalf("reached max retry attempts")
		}
		req, err := http.NewRequest(params.method, tc.endpoint+params.path, nil)
		if err != nil {
			log.Fatalf("error creating http request %s, %s: %v", params.method, tc.endpoint+params.path, err)
		}
		req.Header.Add(traktApiVersionHeaderName, "2")
		req.Header.Add(contentTypeHeaderName, "application/json")
		req.Header.Add(traktApiKeyHeaderName, tc.config.clientId)
		req.Header.Add(authorizationHeaderName, fmt.Sprintf("Bearer %s", tc.config.accessToken))
		if params.body != nil {
			body, err := json.Marshal(params.body)
			if err != nil {
				log.Fatalf("error marshalling request body %s, %s: %v", params.method, tc.endpoint+params.path, err)
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		res, err := tc.client.Do(req)
		if err != nil {
			log.Fatalf("error sending http request %s, %s: %v", params.method, tc.endpoint+params.path, err)
		}
		if res.StatusCode == http.StatusTooManyRequests {
			retryAfterHeader := res.Header.Get("Retry-After")
			retryAfter, err := strconv.Atoi(retryAfterHeader)
			if err != nil {
				log.Fatalf("error converting string %s to integer: %v", retryAfterHeader, err)
			}
			drainBody(res.Body)
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		}
		return res
	}
}

func (tc *traktClient) userIdGet() string {
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktProfilePath,
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt profile: %v", res.StatusCode)
	}
	return readTraktProfile(res.Body)
}

func (tc *traktClient) watchlistItemsGet() []traktItem {
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktWatchlistPath,
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *traktClient) watchlistItemsAdd(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistPath,
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *traktClient) watchlistItemsRemove(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistRemovePath,
		body:   mapTraktItemsToTraktBody(items),
	})
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
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.config.traktUserId, listId),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, errNotFound
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body), nil
}

func (tc *traktClient) listItemsAdd(listId string, items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.config.traktUserId, listId),
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *traktClient) listItemsRemove(listId string, items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsRemovePath, tc.config.traktUserId, listId),
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *traktClient) listsGet() []traktList {
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(traktUserListPath, tc.config.traktUserId, ""),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt lists for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktLists(res.Body)
}

func (tc *traktClient) listAdd(listId, listName string) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListPath, tc.config.traktUserId, ""),
		body: traktListAddBody{
			Name:           listName,
			Description:    fmt.Sprintf("list auto imported from imdb by https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
			Privacy:        "public",
			DisplayNumbers: false,
			AllowComments:  true,
			SortBy:         "rank",
			SortHow:        "asc",
		},
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.config.traktUserId)
}

func (tc *traktClient) listRemove(listId string) {
	res := tc.doRequest(requestParams{
		method: http.MethodDelete,
		path:   fmt.Sprintf(traktUserListPath, tc.config.traktUserId, listId),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusNoContent:
		break
	default:
		log.Fatalf("error removing trakt list %s for user %s: %v", listId, tc.config.traktUserId, res.StatusCode)
	}
	log.Printf("removed trakt list %s for user %s", listId, tc.config.traktUserId)
}

func (tc *traktClient) ratingsGet() []traktItem {
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktRatingsPath,
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *traktClient) ratingsAdd(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktRatingsPath,
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *traktClient) ratingsRemove(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktRatingsRemovePath,
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *traktClient) historyGet(item traktItem) []traktItem {
	var itemId string
	switch item.Type {
	case "movie":
		itemId = item.Movie.Ids.Imdb
	case "show":
		itemId = item.Show.Ids.Imdb
	case "episode":
		itemId = item.Episode.Ids.Imdb
	}
	res := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(traktHistoryGetPath, item.Type+"s", itemId, "1000"),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt history for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *traktClient) historyAdd(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktHistoryPath,
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt history for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func (tc *traktClient) historyRemove(items []traktItem) {
	res := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktHistoryRemovePath,
		body:   mapTraktItemsToTraktBody(items),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt history for user %s: %v", tc.config.traktUserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func mapTraktItemsToTraktBody(items []traktItem) traktListBody {
	body := traktListBody{}
	for _, item := range items {
		switch item.Type {
		case "movie":
			body.Movies = append(body.Movies, item.Movie)
		case "show":
			body.Shows = append(body.Shows, item.Show)
		case "episode":
			body.Episodes = append(body.Episodes, item.Episode)
		default:
			continue
		}
	}
	return body
}

func readTraktProfile(body io.ReadCloser) string {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt profile response: %v", err)
	}
	profile := struct {
		Username string `json:"username"`
	}{}
	err = json.Unmarshal(data, &profile)
	if err != nil {
		log.Fatalf("error unmarshalling trakt profile: %v", err)
	}
	return profile.Username
}

func readTraktLists(body io.ReadCloser) []traktList {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list response: %v", err)
	}
	var traktLists []traktList
	err = json.Unmarshal(data, &traktLists)
	if err != nil {
		log.Fatalf("error unmarshalling trakt lists: %v", err)
	}
	return traktLists
}

func readTraktListItems(body io.ReadCloser) []traktItem {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list items response: %v", err)
	}
	var traktList []traktItem
	err = json.Unmarshal(data, &traktList)
	if err != nil {
		log.Fatalf("error unmarshalling trakt list items: %v", err)
	}
	return traktList
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
