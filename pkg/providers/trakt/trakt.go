package trakt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	authorizationHeaderName = "Authorization"
	contentTypeHeaderName   = "Content-Type"
	AccessTokenKey          = "TRAKT_ACCESS_TOKEN"
	apiKeyHeaderName        = "trakt-api-key"
	apiVersionHeaderName    = "trakt-api-version"
	basePath                = "https://api.trakt.tv/"
	ClientIdKey             = "TRAKT_CLIENT_ID"
	watchlistPath           = "sync/watchlist/"
	watchlistRemovePath     = "sync/watchlist/remove/"
	userListItemsPath       = "users/%s/lists/%s/items/"
	userListItemsRemovePath = "users/%s/lists/%s/items/remove/"
	userListPath            = "users/%s/lists/%s"
	ratingsPath             = "sync/ratings/"
	ratingsRemovePath       = "sync/ratings/remove/"
	profilePath             = "users/me/"
	historyGetPath          = "sync/history/%s/%s?limit=%s"
	historyPath             = "sync/history/"
	historyRemovePath       = "sync/history/remove"
)

type config struct {
	accessToken  string
	clientId     string
	clientSecret string
	UserId       string
}

type Client struct {
	endpoint         string
	client           *http.Client
	Config           config
	retryMaxAttempts int
}

type requestParams struct {
	Method string
	Path   string
	Body   interface{}
}

type Ids struct {
	Imdb string `json:"imdb"`
	Slug string `json:"slug,omitempty"`
}

type ItemSpec struct {
	Ids     Ids    `json:"ids"`
	RatedAt string `json:"rated_at,omitempty"`
	Rating  int    `json:"rating,omitempty"`
}

type Item struct {
	Type      string   `json:"type"`
	RatedAt   string   `json:"rated_at,omitempty"`
	Rating    int      `json:"rating,omitempty"`
	WatchedAt string   `json:"watched_at,omitempty"`
	Movie     ItemSpec `json:"movie,omitempty"`
	Show      ItemSpec `json:"show,omitempty"`
	Episode   ItemSpec `json:"episode,omitempty"`
}

type listBody struct {
	Movies   []ItemSpec `json:"movies,omitempty"`
	Shows    []ItemSpec `json:"shows,omitempty"`
	Episodes []ItemSpec `json:"episodes,omitempty"`
}

type listAddBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Privacy        string `json:"privacy"`
	DisplayNumbers bool   `json:"display_numbers"`
	AllowComments  bool   `json:"allow_comments"`
	SortBy         string `json:"sort_by"`
	SortHow        string `json:"sort_how"`
}

type crudItem struct {
	Movies   int `json:"movies,omitempty"`
	Shows    int `json:"shows,omitempty"`
	Episodes int `json:"episodes,omitempty"`
}

type response struct {
	Item     string   `json:"item,omitempty"`
	Added    crudItem `json:"added,omitempty"`
	Deleted  crudItem `json:"deleted,omitempty"`
	Existing crudItem `json:"existing,omitempty"`
	NotFound listBody `json:"not_found,omitempty"`
}

type list struct {
	Name string `json:"name"`
	Ids  Ids
}

func NewClient() *Client {
	return &Client{
		endpoint: basePath,
		client: &http.Client{
			Timeout: time.Second * 15,
			Transport: &http.Transport{
				IdleConnTimeout: time.Second * 5,
			},
		},
		Config: config{
			accessToken: os.Getenv(AccessTokenKey),
			clientId:    os.Getenv(ClientIdKey),
		},
		retryMaxAttempts: 5,
	}
}

func (tc *Client) doRequest(params requestParams) *http.Response {
	retries := 0
	for {
		if retries == tc.retryMaxAttempts {
			log.Fatalf("reached max retry attempts")
		}
		req, err := http.NewRequest(params.Method, tc.endpoint+params.Path, nil)
		if err != nil {
			log.Fatalf("error creating http request %s, %s: %v", params.Method, tc.endpoint+params.Path, err)
		}
		req.Header.Add(apiVersionHeaderName, "2")
		req.Header.Add(contentTypeHeaderName, "application/json")
		req.Header.Add(apiKeyHeaderName, tc.Config.clientId)
		req.Header.Add(authorizationHeaderName, fmt.Sprintf("Bearer %s", tc.Config.accessToken))
		if params.Body != nil {
			body, err := json.Marshal(params.Body)
			if err != nil {
				log.Fatalf("error marshalling request body %s, %s: %v", params.Method, tc.endpoint+params.Path, err)
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		res, err := tc.client.Do(req)
		if err != nil {
			log.Fatalf("error sending http request %s, %s: %v", params.Method, tc.endpoint+params.Path, err)
		}
		if res.StatusCode == http.StatusTooManyRequests {
			retryAfterHeader := res.Header.Get("Retry-After")
			retryAfter, err := strconv.Atoi(retryAfterHeader)
			if err != nil {
				log.Fatalf("error converting string %s to integer: %v", retryAfterHeader, err)
			}
			client.DrainBody(res.Body)
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		}
		return res
	}
}

func (tc *Client) UserIdGet() string {
	res := tc.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   profilePath,
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt profile: %v", res.StatusCode)
	}
	return readTraktProfile(res.Body)
}

func (tc *Client) WatchlistItemsGet() []Item {
	res := tc.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   watchlistPath,
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) WatchlistItemsAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   watchlistPath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *Client) WatchlistItemsRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   watchlistRemovePath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *Client) ListItemsGet(listId string) ([]Item, error) {
	res := tc.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(userListItemsPath, tc.Config.UserId, listId),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, client.ErrNotFound
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.Config.UserId, res.StatusCode)
	}
	return readTraktListItems(res.Body), nil
}

func (tc *Client) ListItemsAdd(listId string, items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(userListItemsPath, tc.Config.UserId, listId),
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *Client) ListItemsRemove(listId string, items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(userListItemsRemovePath, tc.Config.UserId, listId),
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *Client) ListsGet() []list {
	res := tc.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(userListPath, tc.Config.UserId, ""),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt lists for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	return readTraktLists(res.Body)
}

func (tc *Client) ListAdd(listId, listName string) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(userListPath, tc.Config.UserId, ""),
		Body: listAddBody{
			Name:           listName,
			Description:    fmt.Sprintf("list auto imported from imdb by https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
			Privacy:        "public",
			DisplayNumbers: false,
			AllowComments:  true,
			SortBy:         "rank",
			SortHow:        "asc",
		},
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.Config.UserId, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.Config.UserId)
}

func (tc *Client) ListRemove(listId string) {
	res := tc.doRequest(requestParams{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf(userListPath, tc.Config.UserId, listId),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusNoContent:
		break
	default:
		log.Fatalf("error removing trakt list %s for user %s: %v", listId, tc.Config.UserId, res.StatusCode)
	}
	log.Printf("removed trakt list %s for user %s", listId, tc.Config.UserId)
}

func (tc *Client) RatingsGet() []Item {
	res := tc.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   ratingsPath,
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) RatingsAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   ratingsPath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *Client) RatingsRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   ratingsRemovePath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *Client) HistoryGet(item Item) []Item {
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
		Method: http.MethodGet,
		Path:   fmt.Sprintf(historyGetPath, item.Type+"s", itemId, "1000"),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt history for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) HistoryAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   historyPath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt history for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func (tc *Client) HistoryRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   historyRemovePath,
		Body:   mapTraktItemsToTraktBody(items),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt history for user %s: %v", tc.Config.UserId, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func mapTraktItemsToTraktBody(items []Item) listBody {
	body := listBody{}
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

func readTraktLists(body io.ReadCloser) []list {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list response: %v", err)
	}
	var traktLists []list
	err = json.Unmarshal(data, &traktLists)
	if err != nil {
		log.Fatalf("error unmarshalling trakt lists: %v", err)
	}
	return traktLists
}

func readTraktListItems(body io.ReadCloser) []Item {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list items response: %v", err)
	}
	var traktList []Item
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
	res := response{
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
