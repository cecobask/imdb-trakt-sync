package trakt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvVarKeyClientId        = "TRAKT_CLIENT_ID"
	EnvVarKeyClientSecret    = "TRAKT_CLIENT_SECRET"
	EnvVarKeyPassword        = "TRAKT_PASSWORD"
	EnvVarKeyUsername        = "TRAKT_USERNAME"
	ItemTypeEpisode          = "episode"
	ItemTypeMovie            = "movie"
	ItemTypeShow             = "show"
	formKeyAuthenticityToken = "authenticity_token"
	formKeyCommit            = "commit"
	formKeyUserLogIn         = "user[login]"
	formKeyUserPassword      = "user[password]"
	formKeyUserRemember      = "user[remember_me]"
	headerKeyApiKey          = "trakt-api-key"
	headerKeyApiVersion      = "trakt-api-version"
	headerKeyAuthorization   = "Authorization"
	headerKeyContentLength   = "Content-Length"
	headerKeyContentType     = "Content-Type"
	pathActivate             = "/activate"
	pathActivateAuthorize    = "/activate/authorize"
	pathAuthCodes            = "/oauth/device/code"
	pathAuthSignIn           = "/auth/signin"
	pathAuthTokens           = "/oauth/device/token"
	pathBaseAPI              = "https://api.trakt.tv"
	pathBaseBrowser          = "https://trakt.tv"
	pathHistory              = "/sync/history"
	pathHistoryGet           = "/sync/history/%s/%s?limit=%s"
	pathHistoryRemove        = "/sync/history/remove"
	pathRatings              = "/sync/ratings"
	pathRatingsRemove        = "/sync/ratings/remove"
	pathUserList             = "/users/%s/lists/%s"
	pathUserListItems        = "/users/%s/lists/%s/items"
	pathUserListItemsRemove  = "/users/%s/lists/%s/items/remove"
	pathWatchlist            = "/sync/watchlist"
	pathWatchlistRemove      = "/sync/watchlist/remove"
)

type config struct {
	accessToken  string
	clientId     string
	clientSecret string
	Username     string
	password     string
}

type Client struct {
	endpoint         string
	client           *http.Client
	Config           config
	retryMaxAttempts int
}

type requestParams struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
}

type authCodesBody struct {
	ClientID string `json:"client_id"`
}

type AuthCodesResponse struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
}

type authTokensBody struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type AuthTokensResponse struct {
	AccessToken string `json:"access_token"`
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

func newDefaultClient(endpoint string) *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: time.Second * 20,
			Transport: &http.Transport{
				IdleConnTimeout: time.Second * 20,
			},
			Jar: jar,
		},
		Config: config{
			clientId:     os.Getenv(EnvVarKeyClientId),
			clientSecret: os.Getenv(EnvVarKeyClientSecret),
			Username:     os.Getenv(EnvVarKeyUsername),
			password:     os.Getenv(EnvVarKeyPassword),
		},
		retryMaxAttempts: 5,
	}
	return c
}

func NewClient() *Client {
	c := newDefaultClient(pathBaseAPI)
	authCodes := c.GetAuthCodes()
	doUserAuth(authCodes.UserCode)
	c.Config.accessToken = c.GetAccessToken(authCodes.DeviceCode)
	return c
}

func doUserAuth(userCode string) {
	c := newDefaultClient(pathBaseBrowser)
	c.signIn(c.browseSignIn())
	authenticityToken := c.activate(userCode, c.browseActivate())
	c.activateAuthorize(authenticityToken)
}

func (tc *Client) browseSignIn() string {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+pathAuthSignIn, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer client.DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from trakt response: %v", err)
	}
	authenticityToken, ok := doc.Find("#new_user > input[name=authenticity_token]").Attr("value")
	if !ok {
		log.Fatalf("error scraping trakt authenticity token: authenticity_token not found")
	}
	return authenticityToken
}

func (tc *Client) signIn(authenticityToken string) {
	data := url.Values{}
	data.Set(formKeyAuthenticityToken, authenticityToken)
	data.Set(formKeyUserLogIn, tc.Config.Username)
	data.Set(formKeyUserPassword, tc.Config.password)
	data.Set(formKeyUserRemember, "1")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+pathAuthSignIn, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(headerKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(headerKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer client.DrainBody(res.Body)
}

func (tc *Client) browseActivate() string {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+pathActivate, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer client.DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from trakt response: %v", err)
	}
	authenticityToken, ok := doc.Find("#auth-form-wrapper > form.form-signin > input[name=authenticity_token]").Attr("value")
	if !ok {
		log.Fatalf("error scraping trakt authenticity token: authenticity_token not found")
	}
	return authenticityToken
}

func (tc *Client) activate(userCode, authenticityToken string) string {
	data := url.Values{}
	data.Set(formKeyAuthenticityToken, authenticityToken)
	data.Set("code", userCode)
	data.Set(formKeyCommit, "Continue")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+pathActivate, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(headerKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(headerKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer client.DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from trakt response: %v", err)
	}
	authenticityToken, ok := doc.Find("#auth-form-wrapper > div.form-signin.less-top > div > form:nth-child(1) > input[name=authenticity_token]:nth-child(1)").Attr("value")
	if !ok {
		log.Fatalf("error scraping trakt authenticity token: authenticity_token not found")
	}
	return authenticityToken
}

func (tc *Client) activateAuthorize(authenticityToken string) {
	data := url.Values{}
	data.Set(formKeyAuthenticityToken, authenticityToken)
	data.Set(formKeyCommit, "Yes")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+pathActivateAuthorize, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(headerKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(headerKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer client.DrainBody(res.Body)
}

func (tc *Client) GetAccessToken(deviceCode string) string {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   pathAuthTokens,
		Body: authTokensBody{
			Code:         deviceCode,
			ClientID:     tc.Config.clientId,
			ClientSecret: tc.Config.clientSecret,
		},
		Headers: map[string]string{
			headerKeyContentType: "application/json",
		},
	})
	defer client.DrainBody(res.Body)
	return readAuthTokensResponse(res.Body).AccessToken
}

func (tc *Client) GetAuthCodes() AuthCodesResponse {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   pathAuthCodes,
		Body: authCodesBody{
			ClientID: tc.Config.clientId,
		},
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	return readAuthCodesResponse(res.Body)
}

func (tc *Client) defaultHeaders() map[string]string {
	return map[string]string{
		headerKeyApiVersion:    "2",
		headerKeyContentType:   "application/json",
		headerKeyApiKey:        tc.Config.clientId,
		headerKeyAuthorization: fmt.Sprintf("Bearer %s", tc.Config.accessToken),
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
		for key, value := range params.Headers {
			req.Header.Add(key, value)
		}
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

func (tc *Client) WatchlistItemsGet() []Item {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    pathWatchlist,
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) WatchlistItemsAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathWatchlist,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *Client) WatchlistItemsRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathWatchlistRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *Client) ListItemsGet(listId string) ([]Item, error) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(pathUserListItems, tc.Config.Username, listId),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, client.ErrNotFound
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.Config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body), nil
}

func (tc *Client) ListItemsAdd(listId string, items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(pathUserListItems, tc.Config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *Client) ListItemsRemove(listId string, items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(pathUserListItemsRemove, tc.Config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *Client) ListsGet() []list {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(pathUserList, tc.Config.Username, ""),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt lists for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	return readTraktLists(res.Body)
}

func (tc *Client) ListAdd(listId, listName string) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(pathUserList, tc.Config.Username, ""),
		Body: listAddBody{
			Name:           listName,
			Description:    fmt.Sprintf("list auto imported from imdb by https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
			Privacy:        "public",
			DisplayNumbers: false,
			AllowComments:  true,
			SortBy:         "rank",
			SortHow:        "asc",
		},
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.Config.Username, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.Config.Username)
}

func (tc *Client) ListRemove(listId string) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodDelete,
		Path:    fmt.Sprintf(pathUserList, tc.Config.Username, listId),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusNoContent:
		break
	default:
		log.Fatalf("error removing trakt list %s for user %s: %v", listId, tc.Config.Username, res.StatusCode)
	}
	log.Printf("removed trakt list %s for user %s", listId, tc.Config.Username)
}

func (tc *Client) RatingsGet() []Item {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    pathRatings,
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) RatingsAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathRatings,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *Client) RatingsRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathRatingsRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *Client) HistoryGet(item Item) []Item {
	var itemId string
	switch item.Type {
	case ItemTypeMovie:
		itemId = item.Movie.Ids.Imdb
	case ItemTypeShow:
		itemId = item.Show.Ids.Imdb
	case ItemTypeEpisode:
		itemId = item.Episode.Ids.Imdb
	}
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(pathHistoryGet, item.Type+"s", itemId, "1000"),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt history for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *Client) HistoryAdd(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathHistory,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt history for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func (tc *Client) HistoryRemove(items []Item) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    pathHistoryRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt history for user %s: %v", tc.Config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func mapTraktItemsToTraktBody(items []Item) listBody {
	res := listBody{}
	for _, item := range items {
		switch item.Type {
		case ItemTypeMovie:
			res.Movies = append(res.Movies, item.Movie)
		case ItemTypeShow:
			res.Shows = append(res.Shows, item.Show)
		case ItemTypeEpisode:
			res.Episodes = append(res.Episodes, item.Episode)
		default:
			continue
		}
	}
	return res
}

func readAuthCodesResponse(body io.ReadCloser) AuthCodesResponse {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}
	res := AuthCodesResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt auth codes response: %v", err)
	}
	return res
}

func readAuthTokensResponse(body io.ReadCloser) AuthTokensResponse {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}
	res := AuthTokensResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt auth tokens response: %v", err)
	}
	return res
}

func readTraktLists(body io.ReadCloser) []list {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list response: %v", err)
	}
	var res []list
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt lists: %v", err)
	}
	return res
}

func readTraktListItems(body io.ReadCloser) []Item {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list items response: %v", err)
	}
	var res []Item
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt list items: %v", err)
	}
	return res
}

func readTraktResponse(body io.ReadCloser, item string) {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt response: %v", err)
	}
	res := response{
		Item: item,
	}
	if err = json.Unmarshal(data, &res); err != nil {
		log.Fatalf("failed unmarshalling trakt response")
	}
	prettyPrint, err := json.MarshalIndent(res, "", "    ")
	if err != nil {
		log.Fatalf("failed marshalling trakt response")
	}
	log.Printf("\n%v", string(prettyPrint))
}
