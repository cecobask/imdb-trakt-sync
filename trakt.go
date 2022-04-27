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
	traktClientSecretKey         = "TRAKT_CLIENT_SECRET"
	traktUserIdKey               = "TRAKT_USER_ID"
	traktWatchlistPath           = "sync/watchlist/"
	traktWatchlistRemovePath     = "sync/watchlist/remove"
	traktUserListItemsPath       = "users/%s/lists/%s/items/"
	traktUserListItemsRemovePath = "users/%s/lists/%s/items/remove"
	traktUserListPath            = "users/%s/lists/"
)

type traktClient struct {
	endpoint         string
	client           *http.Client
	credentials      traktCredentials
	retryMaxAttempts int
}

type traktCredentials struct {
	accessToken  string
	clientId     string
	clientSecret string
	traktUserId  string
}

type requestParams struct {
	method string
	path   string
	body   interface{}
}

type Ids struct {
	Imdb string `json:"imdb"`
}

type traktListItemGeneric struct {
	Ids `json:"ids"`
}

type traktListItem struct {
	Type    string               `json:"type"`
	Movie   traktListItemGeneric `json:"movie,omitempty"`
	Show    traktListItemGeneric `json:"show,omitempty"`
	Episode traktListItemGeneric `json:"episode,omitempty"`
}

type customListItemsBody struct {
	Movies   []traktListItemGeneric `json:"movies"`
	Shows    []traktListItemGeneric `json:"shows"`
	Episodes []traktListItemGeneric `json:"episodes"`
}

type listBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Privacy        string `json:"privacy"`
	DisplayNumbers bool   `json:"display_numbers"`
	AllowComments  bool   `json:"allow_comments"`
	SortBy         string `json:"sort_by"`
	SortHow        string `json:"sort_how"`
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
		credentials: traktCredentials{
			accessToken:  os.Getenv(traktAccessTokenKey),
			clientId:     os.Getenv(traktClientIdKey),
			clientSecret: os.Getenv(traktClientSecretKey),
			traktUserId:  os.Getenv(traktUserIdKey),
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
		req.Header.Add(traktApiKeyHeaderName, tc.credentials.clientId)
		req.Header.Add(authorizationHeaderName, fmt.Sprintf("Bearer %s", tc.credentials.accessToken))
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
			log.Println("retrying request...")
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		}
		return res, nil
	}
}

func (tc *traktClient) watchlistItemsGet() []traktListItem {
	log.Println("invoking tc.watchlistItemsGet()")
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   traktWatchlistPath,
	})
	if err != nil {
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.credentials.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *traktClient) watchlistItemsAdd(imdbItems []imdbListItem) {
	if len(imdbItems) == 0 {
		return
	}
	log.Printf("invoking tc.watchlistItemsAdd() (items count: %d)", len(imdbItems))
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistPath,
		body:   mapImdbItemsToListBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.credentials.traktUserId, res.StatusCode)
	}
	readTraktListResponse(res.Body)
}

func (tc *traktClient) watchlistItemsRemove(imdbItems []imdbListItem) {
	if len(imdbItems) == 0 {
		return
	}
	log.Printf("invoking tc.watchlistItemsRemove() (items count: %d)", len(imdbItems))
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   traktWatchlistRemovePath,
		body:   mapImdbItemsToListBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.credentials.traktUserId, res.StatusCode)
	}
	readTraktListResponse(res.Body)
}

func (tc *traktClient) customListItemsGet(listId string) ([]traktListItem, error) {
	log.Printf("invoking tc.customListItemsGet() (list id: %s)", listId)
	res, err := tc.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.credentials.traktUserId, listId),
	})
	if err != nil {
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, errNotFound
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.credentials.traktUserId, res.StatusCode)
	}
	return readTraktListItems(res.Body), nil
}

func (tc *traktClient) customListItemsAdd(listId string, imdbItems []imdbListItem) {
	if len(imdbItems) == 0 {
		return
	}
	log.Printf("invoking tc.customListItemsAdd() (list id: %s, items count: %d)", listId, len(imdbItems))
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsPath, tc.credentials.traktUserId, listId),
		body:   mapImdbItemsToListBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.credentials.traktUserId, res.StatusCode)
	}
	readTraktListResponse(res.Body)
}

func (tc *traktClient) customListItemsRemove(listId string, imdbItems []imdbListItem) {
	if len(imdbItems) == 0 {
		return
	}
	log.Printf("invoking tc.customListItemsRemove() (list id: %s, items count: %d)", listId, len(imdbItems))
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListItemsRemovePath, tc.credentials.traktUserId, listId),
		body:   mapImdbItemsToListBody(imdbItems),
	})
	if err != nil {
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.credentials.traktUserId, res.StatusCode)
	}
	readTraktListResponse(res.Body)
}

func (tc *traktClient) customListAdd(listId, listName string) {
	log.Printf("invoking tc.customListAdd() (list id: %s)", listId)
	res, err := tc.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf(traktUserListPath, tc.credentials.traktUserId),
		body: listBody{
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
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.credentials.traktUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.credentials.traktUserId, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.credentials.traktUserId)
}

func mapImdbItemsToListBody(imdbItems []imdbListItem) customListItemsBody {
	body := customListItemsBody{}
	for _, item := range imdbItems {
		customListItem := traktListItemGeneric{Ids{Imdb: item.ID}}
		switch item.Type {
		case "movie":
			body.Movies = append(body.Movies, customListItem)
		case "tvSeries":
			body.Shows = append(body.Shows, customListItem)
		case "tvMiniSeries":
			body.Shows = append(body.Shows, customListItem)
		case "tvEpisode":
			body.Episodes = append(body.Episodes, customListItem)
		default:
			continue
		}
	}
	return body
}

func readTraktListItems(body io.ReadCloser) []traktListItem {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	var traktListItems []traktListItem
	err = json.Unmarshal(data, &traktListItems)
	if err != nil {
		log.Fatalf("error unmarshalling trakt list items: %v", err)
	}
	return traktListItems
}

func readTraktListResponse(body io.ReadCloser) {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	prettyPrint := bytes.Buffer{}
	if err := json.Indent(&prettyPrint, data, "", "\t"); err != nil {
		log.Fatalf("error formatting json from trakt response: %v", err)
	}
	log.Println(prettyPrint.String())
}
