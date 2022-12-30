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
	endpoint string
	client   *http.Client
	config   TraktConfig
	logger   *zap.Logger
}

type TraktConfig struct {
	accessToken  string
	ClientId     string
	ClientSecret string
	Email        string
	Password     string
	username     string
}

func NewTraktClient(config TraktConfig, logger *zap.Logger) (TraktClientInterface, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	client := &TraktClient{
		endpoint: traktPathBaseAPI,
		client: &http.Client{
			Jar: jar,
		},
		config: config,
		logger: logger,
	}
	if err = client.hydrate(); err != nil {
		return nil, fmt.Errorf("failure hydrating trakt client: %w", err)
	}
	return client, nil
}

func (tc *TraktClient) hydrate() error {
	authCodes, err := tc.GetAuthCodes()
	if err != nil {
		return fmt.Errorf("failure generating auth codes: %w", err)
	}
	authenticityToken, err := tc.BrowseSignIn()
	if err != nil {
		return fmt.Errorf("failure simulating browse to the trakt sign in page: %w", err)
	}
	if err = tc.SignIn(*authenticityToken); err != nil {
		return fmt.Errorf("failure simulating trakt sign in form submission: %w", err)
	}
	authenticityToken, err = tc.BrowseActivate()
	if err != nil {
		return fmt.Errorf("failure simulating browse to the trakt device activation page: %w", err)
	}
	authenticityToken, err = tc.Activate(authCodes.UserCode, *authenticityToken)
	if err != nil {
		return fmt.Errorf("failure simulating trakt device activation form submission: %w", err)
	}
	if err = tc.ActivateAuthorize(*authenticityToken); err != nil {
		return fmt.Errorf("failure simulating trakt api app allowlisting: %w", err)
	}
	authTokens, err := tc.GetAccessToken(authCodes.DeviceCode)
	if err != nil {
		return fmt.Errorf("failure exchanging trakt device code for access token: %w", err)
	}
	tc.config.accessToken = authTokens.AccessToken
	return nil
}

func (tc *TraktClient) BrowseSignIn() (*string, error) {
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseBrowser,
		Path:     traktPathAuthSignIn,
		Url:      traktPathBaseBrowser + traktPathAuthSignIn,
		Body:     http.NoBody,
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return scrapeSelectionAttribute(response.Body, clientNameTrakt, "#new_user > input[name=authenticity_token]", "value")
}

func (tc *TraktClient) SignIn(authenticityToken string) error {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyUserLogIn, tc.config.Email)
	data.Set(traktFormKeyUserPassword, tc.config.Password)
	data.Set(traktFormKeyUserRemember, "1")
	encodedData := data.Encode()
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseBrowser,
		Path:     traktPathAuthSignIn,
		Url:      traktPathBaseBrowser + traktPathAuthSignIn,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}

func (tc *TraktClient) BrowseActivate() (*string, error) {
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseBrowser,
		Path:     traktPathActivate,
		Url:      traktPathBaseBrowser + traktPathActivate,
		Body:     http.NoBody,
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return scrapeSelectionAttribute(response.Body, clientNameTrakt, "#auth-form-wrapper > form.form-signin > input[name=authenticity_token]", "value")
}

func (tc *TraktClient) Activate(userCode, authenticityToken string) (*string, error) {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyCode, userCode)
	data.Set(traktFormKeyCommit, "Continue")
	encodedData := data.Encode()
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseBrowser,
		Path:     traktPathActivate,
		Url:      traktPathBaseBrowser + traktPathActivate,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return scrapeSelectionAttribute(response.Body, clientNameTrakt, "#auth-form-wrapper > div.form-signin.less-top > div > form:nth-child(1) > input[name=authenticity_token]:nth-child(1)", "value")
}

func (tc *TraktClient) ActivateAuthorize(authenticityToken string) error {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyCommit, "Yes")
	encodedData := data.Encode()
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseBrowser,
		Path:     traktPathActivateAuthorize,
		Url:      traktPathBaseBrowser + traktPathActivateAuthorize,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	value, err := scrapeSelectionAttribute(response.Body, clientNameTrakt, "#desktop-user-avatar", "href")
	if err != nil {
		return err
	}
	hrefPieces := strings.Split(*value, "/")
	if len(hrefPieces) != 3 {
		return fmt.Errorf("failure scraping trakt username")
	}
	tc.config.username = hrefPieces[2]
	return nil
}

func (tc *TraktClient) GetAccessToken(deviceCode string) (*entities.TraktAuthTokensResponse, error) {
	body, err := json.Marshal(entities.TraktAuthTokensBody{
		Code:         deviceCode,
		ClientID:     tc.config.ClientId,
		ClientSecret: tc.config.ClientSecret,
	})
	if err != nil {
		return nil, err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathAuthTokens,
		Url:      traktPathBaseAPI + traktPathAuthTokens,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers: map[string]string{
			traktHeaderKeyContentType: "application/json",
		},
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return readAuthTokensResponse(response.Body)
}

func (tc *TraktClient) GetAuthCodes() (*entities.TraktAuthCodesResponse, error) {
	body, err := json.Marshal(entities.TraktAuthCodesBody{ClientID: tc.config.ClientId})
	if err != nil {
		return nil, err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathAuthCodes,
		Url:      traktPathBaseAPI + traktPathAuthCodes,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return readAuthCodesResponse(response.Body)
}

func (tc *TraktClient) defaultApiHeaders() map[string]string {
	return map[string]string{
		traktHeaderKeyApiVersion:    "2",
		traktHeaderKeyContentType:   "application/json",
		traktHeaderKeyApiKey:        tc.config.ClientId,
		traktHeaderKeyAuthorization: fmt.Sprintf("Bearer %s", tc.config.accessToken),
	}
}

func (tc *TraktClient) doRequest(reqFields entities.RequestFields) (*http.Response, error) {
	retries := 0
	for {
		if retries == 5 {
			return nil, fmt.Errorf("reached max retry attempts")
		}
		tc.endpoint = reqFields.Endpoint
		req, err := http.NewRequest(reqFields.Method, reqFields.Url, reqFields.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating http request %s %s: %w", reqFields.Method, reqFields.Url, err)
		}
		for key, value := range reqFields.Headers {
			req.Header.Set(key, value)
		}
		response, err := tc.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending http request %s, %s: %w", req.Method, req.URL, err)
		}
		switch response.StatusCode {
		case http.StatusOK:
			return response, nil
		case http.StatusCreated:
			return response, nil
		case http.StatusNoContent:
			return response, nil
		case http.StatusNotFound:
			return response, nil // handled individually in various functions
		case traktStatusCodeEnhanceYourCalm:
			response.Body.Close()
			return nil, &ApiError{
				clientName: clientNameTrakt,
				httpMethod: response.Request.Method,
				url:        response.Request.URL.String(),
				StatusCode: response.StatusCode,
				details:    fmt.Sprintf("trakt account limit exceeded, more info here: %s", "https://github.com/trakt/api-help/discussions/350"),
			}
		case http.StatusTooManyRequests:
			response.Body.Close()
			retryAfter, err := strconv.Atoi(response.Header.Get(traktHeaderKeyRetryAfter))
			if err != nil {
				return nil, fmt.Errorf("failure parsing the value of trakt header %s to integer: %w", traktHeaderKeyRetryAfter, err)
			}
			time.Sleep(time.Duration(retryAfter) * time.Second)
			retries++
			continue
		default:
			response.Body.Close()
			return nil, &ApiError{
				clientName: clientNameTrakt,
				httpMethod: response.Request.Method,
				url:        response.Request.URL.String(),
				StatusCode: response.StatusCode,
				details:    fmt.Sprintf("unexpected status code %d", response.StatusCode),
			}
		}
	}
}

func (tc *TraktClient) WatchlistGet() (*entities.TraktList, error) {
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathWatchlist,
		Url:      traktPathBaseAPI + traktPathWatchlist,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	list := entities.TraktList{
		Ids: entities.TraktIds{
			Slug: "watchlist",
		},
		IsWatchlist: true,
	}
	return readTraktListResponse(response.Body, list)
}

func (tc *TraktClient) WatchlistItemsAdd(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathWatchlist,
		Url:      traktPathBaseAPI + traktPathWatchlist,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", zap.Object("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) WatchlistItemsRemove(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathWatchlistRemove,
		Url:      traktPathBaseAPI + traktPathWatchlistRemove,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", zap.Object("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) ListGet(listId string) (*entities.TraktList, error) {
	path := fmt.Sprintf(traktPathUserListItems, tc.config.username, listId)
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, &ApiError{
			clientName: clientNameTrakt,
			httpMethod: response.Request.Method,
			url:        response.Request.URL.String(),
			StatusCode: response.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", listId),
		}
	}
	list := entities.TraktList{
		Ids: entities.TraktIds{
			Slug: listId,
		},
	}
	return readTraktListResponse(response.Body, list)
}

func (tc *TraktClient) ListItemsAdd(listId string, items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	path := fmt.Sprintf(traktPathUserListItems, tc.config.username, listId)
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", zap.Object(listId, traktResponse))
	return nil
}

func (tc *TraktClient) ListItemsRemove(listId string, items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	path := fmt.Sprintf(traktPathUserListItemsRemove, tc.config.username, listId)
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", zap.Object(listId, traktResponse))
	return nil
}

func (tc *TraktClient) ListsGet() ([]entities.TraktList, error) {
	path := fmt.Sprintf(traktPathUserList, tc.config.username, "")
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return readTraktLists(response.Body)
}

func (tc *TraktClient) ListAdd(listId, listName string) error {
	body, err := json.Marshal(entities.TraktListAddBody{
		Name:           listName,
		Description:    fmt.Sprintf("list auto imported from imdb by https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
		Privacy:        "public",
		DisplayNumbers: false,
		AllowComments:  true,
		SortBy:         "rank",
		SortHow:        "asc",
	})
	if err != nil {
		return err
	}
	path := fmt.Sprintf(traktPathUserList, tc.config.username, "")
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	response.Body.Close()
	tc.logger.Info(fmt.Sprintf("created trakt list %s", listId))
	return nil
}

func (tc *TraktClient) ListRemove(listId string) error {
	path := fmt.Sprintf(traktPathUserList, tc.config.username, listId)
	requestFields := entities.RequestFields{
		Method:   http.MethodDelete,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	response.Body.Close()
	tc.logger.Info(fmt.Sprintf("removed trakt list %s", listId))
	return nil
}

func (tc *TraktClient) RatingsGet() ([]entities.TraktItem, error) {
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathRatings,
		Url:      traktPathBaseAPI + traktPathRatings,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return readTraktItems(response.Body)
}

func (tc *TraktClient) RatingsAdd(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathRatings,
		Url:      traktPathBaseAPI + traktPathRatings,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", zap.Object("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) RatingsRemove(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathRatingsRemove,
		Url:      traktPathBaseAPI + traktPathRatingsRemove,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", zap.Object("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryGet(itemType, itemId string) ([]entities.TraktItem, error) {
	path := fmt.Sprintf(traktPathHistoryGet, itemType+"s", itemId, "1000")
	requestFields := entities.RequestFields{
		Method:   http.MethodGet,
		Endpoint: traktPathBaseAPI,
		Path:     path,
		Url:      traktPathBaseAPI + path,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return nil, err
	}
	return readTraktItems(response.Body)
}

func (tc *TraktClient) HistoryAdd(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathHistory,
		Url:      traktPathBaseAPI + traktPathHistory,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt history", zap.Object("history", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryRemove(items []entities.TraktItem) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	requestFields := entities.RequestFields{
		Method:   http.MethodPost,
		Endpoint: traktPathBaseAPI,
		Path:     traktPathHistoryRemove,
		Url:      traktPathBaseAPI + traktPathHistoryRemove,
		Body:     io.NopCloser(bytes.NewReader(body)),
		Headers:  tc.defaultApiHeaders(),
	}
	response, err := tc.doRequest(requestFields)
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
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
	defer body.Close()
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
	defer body.Close()
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
	defer body.Close()
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

func readTraktItems(body io.ReadCloser) ([]entities.TraktItem, error) {
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	var items []entities.TraktItem
	if err = json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt list: %w", err)
	}
	return items, nil
}

func readTraktListResponse(body io.ReadCloser, list entities.TraktList) (*entities.TraktList, error) {
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failure reading response body: %w", err)
	}
	if err = json.Unmarshal(data, &list.ListItems); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt list: %w", err)
	}
	return &list, nil
}

func readTraktResponse(body io.ReadCloser) (*entities.TraktResponse, error) {
	defer body.Close()
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

func scrapeSelectionAttribute(body io.ReadCloser, clientName, selector, attribute string) (*string, error) {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from %s response: %w", clientName, err)
	}
	value, ok := doc.Find(selector).Attr(attribute)
	if !ok {
		return nil, fmt.Errorf("failure scraping trakt response for selector %s and attribute %s", selector, attribute)
	}
	return &value, nil
}
