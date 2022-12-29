package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	traktFormKeyAuthenticityToken = "authenticity_token"
	traktFormKeyCode              = "code"
	traktFormKeyCommit            = "commit"
	traktFormKeyUserLogIn         = "user[login]"
	traktFormKeyUserPassword      = "user[password]"
	traktFormKeyUserRemember      = "user[remember_me]"

	traktHeaderKeyApiKey        = "trakt-api-key"
	traktHeaderKeyApiVersion    = "trakt-api-version"
	traktHeaderKeyAuthorization = "Authorization"
	traktHeaderKeyContentLength = "Content-Length"
	traktHeaderKeyContentType   = "Content-Type"
	traktHeaderKeyRetryAfter    = "Retry-After"

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

	traktStatusCodeEnhanceYourCalm = 420 // https://github.com/trakt/api-help/discussions/350
)

type TraktClient struct {
	endpoint         string
	client           *http.Client
	config           TraktConfig
	retryMaxAttempts int
	logger           *zap.Logger
}

type TraktConfig struct {
	accessToken  string
	ClientId     string
	ClientSecret string
	Email        string
	Password     string
	Username     string
}

type requestParams struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
}

func newDefaultClient(endpoint string, config TraktConfig, logger *zap.Logger) (*TraktClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	c := &TraktClient{
		endpoint: endpoint,
		client: &http.Client{
			Jar: jar,
		},
		config: config,
		logger: logger,
	}
	return c, nil
}

func NewTraktClient(config TraktConfig, logger *zap.Logger) (TraktClientInterface, error) {
	apiClient, err := newDefaultClient(traktPathBaseAPI, config, logger)
	if err != nil {
		return nil, fmt.Errorf("failure creating trakt api client: %w", err)
	}
	authCodes, err := apiClient.GetAuthCodes()
	if err != nil {
		return nil, fmt.Errorf("failure generating auth codes: %w", err)
	}
	browserClient, err := newDefaultClient(traktPathBaseBrowser, config, logger)
	if err != nil {
		return nil, fmt.Errorf("failure creating trakt browser client: %w", err)
	}
	username, err := doUserAuth(authCodes.UserCode, browserClient)
	if err != nil {
		return nil, fmt.Errorf("failure performing user authentication flow: %w", err)
	}
	apiClient.config.Username = *username
	authTokens, err := apiClient.GetAccessToken(authCodes.DeviceCode)
	if err != nil {
		return nil, fmt.Errorf("failure exchanging trakt device code for access token: %w", err)
	}
	apiClient.config.accessToken = authTokens.AccessToken
	return apiClient, nil
}

func doUserAuth(userCode string, tc *TraktClient) (*string, error) {
	authenticityToken, err := tc.BrowseSignIn()
	if err != nil {
		return nil, fmt.Errorf("failure simulating browse to the trakt sign in page: %w", err)
	}
	if err = tc.SignIn(*authenticityToken); err != nil {
		return nil, fmt.Errorf("failure simulating trakt sign in form submission: %w", err)
	}
	authenticityToken, err = tc.BrowseActivate()
	if err != nil {
		return nil, fmt.Errorf("failure simulating browse to the trakt device activation page: %w", err)
	}
	authenticityToken, err = tc.Activate(userCode, *authenticityToken)
	if err != nil {
		return nil, fmt.Errorf("failure simulating trakt device activation form submission: %w", err)
	}
	username, err := tc.ActivateAuthorize(*authenticityToken)
	if err != nil {
		return nil, fmt.Errorf("failure simulating trakt api app allowlisting: %w", err)
	}
	return username, nil
}

func scrapeSelectionAttribute(body io.ReadCloser, selector, attribute string) (*string, error) {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from trakt response: %w", err)
	}
	value, ok := doc.Find(selector).Attr(attribute)
	if !ok {
		return nil, fmt.Errorf("failure scraping trakt response for selector %s and attribute %s", selector, attribute)
	}
	return &value, nil
}

func (tc *TraktClient) BrowseSignIn() (*string, error) {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+traktPathAuthSignIn, nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", http.MethodGet, tc.endpoint+traktPathAuthSignIn, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", req.Method, req.URL, err)
	}
	return scrapeSelectionAttribute(res.Body, "#new_user > input[name=authenticity_token]", "value")
}

func (tc *TraktClient) SignIn(authenticityToken string) error {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyUserLogIn, tc.config.Email)
	data.Set(traktFormKeyUserPassword, tc.config.Password)
	data.Set(traktFormKeyUserRemember, "1")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathAuthSignIn, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failure creating http request %s %s: %w", http.MethodPost, tc.endpoint+traktPathAuthSignIn, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		return fmt.Errorf("failure sending http request %s %s: %w", req.Method, req.URL, err)
	}
	defer res.Body.Close()
	return nil
}

func (tc *TraktClient) BrowseActivate() (*string, error) {
	req, err := http.NewRequest(http.MethodGet, tc.endpoint+traktPathActivate, nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", http.MethodGet, tc.endpoint+traktPathActivate, err)
	}
	res, err := tc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", req.Method, req.URL, err)
	}
	return scrapeSelectionAttribute(res.Body, "#auth-form-wrapper > form.form-signin > input[name=authenticity_token]", "value")
}

func (tc *TraktClient) Activate(userCode, authenticityToken string) (*string, error) {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyCode, userCode)
	data.Set(traktFormKeyCommit, "Continue")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathActivate, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", http.MethodPost, tc.endpoint+traktPathActivate, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", req.Method, req.URL, err)
	}
	return scrapeSelectionAttribute(res.Body, "#auth-form-wrapper > div.form-signin.less-top > div > form:nth-child(1) > input[name=authenticity_token]:nth-child(1)", "value")
}

func (tc *TraktClient) ActivateAuthorize(authenticityToken string) (*string, error) {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyCommit, "Yes")
	req, err := http.NewRequest(http.MethodPost, tc.endpoint+traktPathActivateAuthorize, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", http.MethodPost, tc.endpoint+traktPathActivateAuthorize, err)
	}
	req.Header.Add(traktHeaderKeyContentType, "application/x-www-form-urlencoded")
	req.Header.Add(traktHeaderKeyContentLength, strconv.Itoa(len(data.Encode())))
	res, err := tc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", req.Method, req.URL, err)
	}
	value, err := scrapeSelectionAttribute(res.Body, "#desktop-user-avatar", "href")
	if err != nil {
		return nil, err
	}
	hrefPieces := strings.Split(*value, "/")
	if len(hrefPieces) != 3 {
		return nil, fmt.Errorf("failure scraping trakt username")
	}
	return &hrefPieces[2], nil
}

func (tc *TraktClient) GetAccessToken(deviceCode string) (*entities.TraktAuthTokensResponse, error) {
	res, err := tc.doRequest(requestParams{
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
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return readAuthTokensResponse(res.Body)
}

func (tc *TraktClient) GetAuthCodes() (*entities.TraktAuthCodesResponse, error) {
	res, err := tc.doRequest(requestParams{
		Method: http.MethodPost,
		Path:   traktPathAuthCodes,
		Body: entities.TraktAuthCodesBody{
			ClientID: tc.config.ClientId,
		},
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
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

func (tc *TraktClient) doRequest(params requestParams) (*http.Response, error) {
	retries := 0
	for {
		if retries == 5 {
			return nil, fmt.Errorf("reached max retry attempts")
		}
		req, err := http.NewRequest(params.Method, tc.endpoint+params.Path, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating http request %s %s: %w", params.Method, tc.endpoint+params.Path, err)
		}
		for key, value := range params.Headers {
			req.Header.Add(key, value)
		}
		if params.Body != nil {
			body, err := json.Marshal(params.Body)
			if err != nil {
				return nil, fmt.Errorf("error marshalling request body %s %s: %w", req.Method, req.URL, err)
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		res, err := tc.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending http request %s, %s: %w", req.Method, req.URL, err)
		}
		switch res.StatusCode {
		case http.StatusOK:
			return res, nil
		case http.StatusCreated:
			return res, nil
		case http.StatusNoContent:
			return res, nil
		case http.StatusNotFound:
			return res, nil // handled individually in various functions
		case traktStatusCodeEnhanceYourCalm:
			res.Body.Close()
			return nil, &ApiError{
				clientName: clientNameTrakt,
				httpMethod: res.Request.Method,
				url:        res.Request.URL.String(),
				StatusCode: res.StatusCode,
				details:    fmt.Sprintf("trakt account limit exceeded, more info here: %s", "https://github.com/trakt/api-help/discussions/350"),
			}
		case http.StatusTooManyRequests:
			res.Body.Close()
			retryAfter, err := strconv.Atoi(res.Header.Get(traktHeaderKeyRetryAfter))
			if err != nil {
				return nil, fmt.Errorf("failure parsing the value of trakt header %s to integer: %w", traktHeaderKeyRetryAfter, err)
			}
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		default:
			res.Body.Close()
			return nil, &ApiError{
				clientName: clientNameTrakt,
				httpMethod: res.Request.Method,
				url:        res.Request.URL.String(),
				StatusCode: res.StatusCode,
				details:    fmt.Sprintf("unexpected status code %d", res.StatusCode),
			}
		}
	}
}

func (tc *TraktClient) WatchlistItemsGet() ([]entities.TraktItem, error) {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    traktPathWatchlist,
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) WatchlistItemsAdd(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathWatchlist,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", zap.Object("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) WatchlistItemsRemove(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathWatchlistRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", zap.Object("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) ListItemsGet(listId string) ([]entities.TraktItem, error) {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(traktPathUserListItems, tc.config.Username, listId),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, &ApiError{
			clientName: clientNameTrakt,
			httpMethod: res.Request.Method,
			url:        res.Request.URL.String(),
			StatusCode: res.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", listId),
		}
	}
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) ListItemsAdd(listId string, items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(traktPathUserListItems, tc.config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", zap.Object(listId, traktResponse))
	return nil
}

func (tc *TraktClient) ListItemsRemove(listId string, items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf(traktPathUserListItemsRemove, tc.config.Username, listId),
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", zap.Object(listId, traktResponse))
	return nil
}

func (tc *TraktClient) ListsGet() ([]entities.TraktList, error) {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(traktPathUserList, tc.config.Username, ""),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return readTraktLists(res.Body)
}

func (tc *TraktClient) ListAdd(listId, listName string) error {
	res, err := tc.doRequest(requestParams{
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
	if err != nil {
		return err
	}
	defer res.Body.Close()
	tc.logger.Info(fmt.Sprintf("created trakt list %s", listId))
	return nil
}

func (tc *TraktClient) ListRemove(listId string) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodDelete,
		Path:    fmt.Sprintf(traktPathUserList, tc.config.Username, listId),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	tc.logger.Info(fmt.Sprintf("removed trakt list %s", listId))
	return nil
}

func (tc *TraktClient) RatingsGet() ([]entities.TraktItem, error) {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    traktPathRatings,
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) RatingsAdd(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathRatings,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", zap.Object("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) RatingsRemove(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathRatingsRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", zap.Object("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryGet(itemType, itemId string) ([]entities.TraktItem, error) {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf(traktPathHistoryGet, itemType+"s", itemId, "1000"),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return readTraktListItems(res.Body)
}

func (tc *TraktClient) HistoryAdd(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathHistory,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt history", zap.Object("history", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryRemove(items []entities.TraktItem) error {
	res, err := tc.doRequest(requestParams{
		Method:  http.MethodPost,
		Path:    traktPathHistoryRemove,
		Body:    mapTraktItemsToTraktBody(items),
		Headers: tc.defaultHeaders(),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	traktResponse, err := readTraktResponse(res.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt history", zap.Object("history", traktResponse))
	return nil
}

func mapTraktItemsToTraktBody(items []entities.TraktItem) entities.TraktListBody {
	res := entities.TraktListBody{}
	for i := range items {
		switch items[i].Type {
		case entities.TraktItemTypeMovie:
			res.Movies = append(res.Movies, items[i].Movie)
		case entities.TraktItemTypeShow:
			res.Shows = append(res.Shows, items[i].Show)
		case entities.TraktItemTypeEpisode:
			res.Episodes = append(res.Episodes, items[i].Episode)
		default:
			continue
		}
	}
	return res
}

func readAuthCodesResponse(body io.ReadCloser) (*entities.TraktAuthCodesResponse, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	res := entities.TraktAuthCodesResponse{}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt auth codes response: %w", err)
	}
	return &res, nil
}

func readAuthTokensResponse(body io.ReadCloser) (*entities.TraktAuthTokensResponse, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	res := entities.TraktAuthTokensResponse{}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt auth tokens response: %w", err)
	}
	return &res, nil
}

func readTraktLists(body io.ReadCloser) ([]entities.TraktList, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	var res []entities.TraktList
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt lists: %w", err)
	}
	return res, nil
}

func readTraktListItems(body io.ReadCloser) ([]entities.TraktItem, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	var res []entities.TraktItem
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt list items: %w", err)
	}
	return res, nil
}

func readTraktResponse(body io.ReadCloser) (*entities.TraktResponse, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading trakt response body: %w", err)
	}
	res := entities.TraktResponse{}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt response: %w", err)
	}
	return &res, nil
}
