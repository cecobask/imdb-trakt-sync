package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	traktFormKeyAuthenticityToken = "authenticity_token"
	traktFormKeyCommit            = "commit"
	traktFormKeyUserLogIn         = "user[login]"
	traktFormKeyUserPassword      = "user[password]"
	traktFormKeyUserRemember      = "user[remember_me]"

	traktHeaderKeyApiKey        = "trakt-api-key"
	traktHeaderKeyApiVersion    = "trakt-api-version"
	traktHeaderKeyAuthorization = "Authorization"
	traktHeaderKeyContentLength = "Content-Length"
	traktHeaderKeyContentType   = "Content-Type"

	traktPathActivate            = "/activate"
	traktPathActivateAuthorize   = "/activate/authorize"
	traktPathAuthCodes           = "/oauth/device/code"
	traktPathAuthSignIn          = "/auth/signin"
	traktPathAuthTokens          = "/oauth/device/token"
	traktPathBaseAPI             = "https://api.trakt.tv"
	traktPathBaseBrowser         = "https://trakt.tv"
	traktPathHistory             = "/sync/history"
	traktPathHistoryGet          = "/sync/history/%s/%s?limit=%s"
	traktPathHistoryRemove       = "/sync/history/remove"
	traktPathRatings             = "/sync/ratings"
	traktPathRatingsRemove       = "/sync/ratings/remove"
	traktPathUserList            = "/users/%s/lists/%s"
	traktPathUserListItems       = "/users/%s/lists/%s/items"
	traktPathUserListItemsRemove = "/users/%s/lists/%s/items/remove"
	traktPathWatchlist           = "/sync/watchlist"
	traktPathWatchlistRemove     = "/sync/watchlist/remove"
)

type TraktClient struct {
	endpoint         string
	client           *http.Client
	config           TraktConfig
	retryMaxAttempts int
}

type TraktConfig struct {
	accessToken  string
	ClientId     string
	ClientSecret string
	Username     string
	Password     string
}

type requestParams struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
}

func newDefaultClient(endpoint string, config TraktConfig) *TraktClient {
	jar, _ := cookiejar.New(nil)
	c := &TraktClient{
		endpoint: endpoint,
		client: &http.Client{
			Jar: jar,
		},
		config: config,
	}
	return c
}

func NewTraktClient(config TraktConfig) TraktClientInterface {
	c := newDefaultClient(traktPathBaseAPI, config)
	authCodes := c.GetAuthCodes()
	doUserAuth(authCodes.UserCode, config)
	c.config.accessToken = c.GetAccessToken(authCodes.DeviceCode)
	return c
}

func doUserAuth(userCode string, config TraktConfig) {
	c := newDefaultClient(traktPathBaseBrowser, config)
	c.SignIn(c.BrowseSignIn())
	authenticityToken := c.Activate(userCode, c.BrowseActivate())
	c.ActivateAuthorize(authenticityToken)
}

func (tc *TraktClient) BrowseSignIn() string {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+traktPathAuthSignIn, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer DrainBody(res.Body)
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

func (tc *TraktClient) SignIn(authenticityToken string) {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyUserLogIn, tc.config.Username)
	data.Set(traktFormKeyUserPassword, tc.config.Password)
	data.Set(traktFormKeyUserRemember, "1")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathAuthSignIn, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer DrainBody(res.Body)
}

func (tc *TraktClient) BrowseActivate() string {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+traktPathActivate, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer DrainBody(res.Body)
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

func (tc *TraktClient) Activate(userCode, authenticityToken string) string {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set("code", userCode)
	data.Set(traktFormKeyCommit, "Continue")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathActivate, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer DrainBody(res.Body)
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

func (tc *TraktClient) ActivateAuthorize(authenticityToken string) {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyCommit, "Yes")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathActivateAuthorize, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", req.Method, req.URL, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", req.Method, req.URL, err)
	}
	defer DrainBody(res.Body)
}

func (tc *TraktClient) GetAccessToken(deviceCode string) string {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   traktPathAuthTokens,
		Body: entities.TraktAuthTokensBody{
			Code:         deviceCode,
			ClientID:     tc.config.ClientId,
			ClientSecret: tc.config.ClientSecret,
		},
		Headers: map[string]string{
			traktHeaderKeyContentType: "application/json",
		},
	})
	defer DrainBody(res.Body)
	return readAuthTokensResponse(res.Body).AccessToken
}

func (tc *TraktClient) GetAuthCodes() entities.TraktAuthCodesResponse {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   traktPathAuthCodes,
		Body: entities.TraktAuthCodesBody{
			ClientID: tc.config.ClientId,
		},
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	return readAuthCodesResponse(res.Body)
}

func (tc *TraktClient) defaultHeaders() map[string]string {
	return map[string]string{
		traktHeaderKeyApiVersion:    "2",
		traktHeaderKeyContentType:   "application/json",
		traktHeaderKeyApiKey:        tc.config.ClientId,
		traktHeaderKeyAuthorization: fmt.Sprintf("Bearer %s", tc.config.accessToken),
	}
}

func (tc *TraktClient) doRequest(params requestParams) *http.Response {
	retries := 0
	for {
		if retries == 5 {
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
			DrainBody(res.Body)
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		}
		return res
	}
}

func (tc *TraktClient) WatchlistItemsGet() []entities.TraktItem {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    traktPathWatchlist,
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt watchlist for user %s: %v", tc.config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) WatchlistItemsAdd(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathWatchlist,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt watchlist by user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *TraktClient) WatchlistItemsRemove(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathWatchlistRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt watchlist by user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "watchlist")
}

func (tc *TraktClient) ListItemsGet(listId string) ([]entities.TraktItem, error) {
	path := fmt.Sprintf(traktPathUserListItems, tc.config.Username, listId)
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    path,
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil, &ResourceNotFoundError{
			resourceType: resourceTypeList,
			resourceName: listId,
			httpMethod:   http.MethodGet,
			url:          path,
		}
	default:
		log.Fatalf("error retrieving trakt list items from %s by user %s: %v", listId, tc.config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body), nil
}

func (tc *TraktClient) ListItemsAdd(listId string, items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(traktPathUserListItems, tc.config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding items to trakt list %s by user %s: %v", listId, tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *TraktClient) ListItemsRemove(listId string, items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(traktPathUserListItemsRemove, tc.config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing items from trakt list %s by user %s: %v", listId, tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, listId)
}

func (tc *TraktClient) ListsGet() []entities.TraktList {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(traktPathUserList, tc.config.Username, ""),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error retrieving trakt lists for user %s: %v", tc.config.Username, res.StatusCode)
	}
	return readTraktLists(res.Body)
}

func (tc *TraktClient) ListAdd(listId, listName string) {
	res := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(traktPathUserList, tc.config.Username, ""),
		Body: entities.TraktListAddBody{
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
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error creating trakt list %s for user %s: %v", listId, tc.config.Username, res.StatusCode)
	}
	log.Printf("created trakt list %s for user %s", listId, tc.config.Username)
}

func (tc *TraktClient) ListRemove(listId string) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodDelete,
		Path:    fmt.Sprintf(traktPathUserList, tc.config.Username, listId),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusNoContent:
		break
	default:
		log.Fatalf("error removing trakt list %s for user %s: %v", listId, tc.config.Username, res.StatusCode)
	}
	log.Printf("removed trakt list %s for user %s", listId, tc.config.Username)
}

func (tc *TraktClient) RatingsGet() []entities.TraktItem {
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    traktPathRatings,
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt ratings for user %s: %v", tc.config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) RatingsAdd(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathRatings,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt ratings for user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *TraktClient) RatingsRemove(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathRatingsRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt ratings for user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "ratings")
}

func (tc *TraktClient) HistoryGet(item entities.TraktItem) []entities.TraktItem {
	var itemId string
	switch item.Type {
	case entities.TraktItemTypeMovie:
		itemId = item.Movie.Ids.Imdb
	case entities.TraktItemTypeShow:
		itemId = item.Show.Ids.Imdb
	case entities.TraktItemTypeEpisode:
		itemId = item.Episode.Ids.Imdb
	}
	res := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(traktPathHistoryGet, item.Type+"s", itemId, "1000"),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusNotFound:
		return nil
	default:
		log.Fatalf("error retrieving trakt history for user %s: %v", tc.config.Username, res.StatusCode)
	}
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) HistoryAdd(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathHistory,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusCreated:
		break
	default:
		log.Fatalf("error adding trakt history for user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func (tc *TraktClient) HistoryRemove(items []entities.TraktItem) {
	res := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathHistoryRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	defer DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	default:
		log.Fatalf("error removing trakt history for user %s: %v", tc.config.Username, res.StatusCode)
	}
	readTraktResponse(res.Body, "history")
}

func mapTraktItemsToTraktBody(items []entities.TraktItem) entities.TraktListBody {
	res := entities.TraktListBody{}
	for _, item := range items {
		switch item.Type {
		case entities.TraktItemTypeMovie:
			res.Movies = append(res.Movies, item.Movie)
		case entities.TraktItemTypeShow:
			res.Shows = append(res.Shows, item.Show)
		case entities.TraktItemTypeEpisode:
			res.Episodes = append(res.Episodes, item.Episode)
		default:
			continue
		}
	}
	return res
}

func readAuthCodesResponse(body io.ReadCloser) entities.TraktAuthCodesResponse {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}
	res := entities.TraktAuthCodesResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt auth codes response: %v", err)
	}
	return res
}

func readAuthTokensResponse(body io.ReadCloser) entities.TraktAuthTokensResponse {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}
	res := entities.TraktAuthTokensResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt auth tokens response: %v", err)
	}
	return res
}

func readTraktLists(body io.ReadCloser) []entities.TraktList {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list response: %v", err)
	}
	var res []entities.TraktList
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("error unmarshalling trakt lists: %v", err)
	}
	return res
}

func readTraktListItems(body io.ReadCloser) []entities.TraktItem {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Fatalf("error reading trakt list items response: %v", err)
	}
	var res []entities.TraktItem
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
	res := entities.TraktResponse{
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
