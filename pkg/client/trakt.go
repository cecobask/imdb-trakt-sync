package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	appconfig "github.com/cecobask/imdb-trakt-sync/pkg/config"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
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
	client *http.Client
	config traktConfig
	logger *slog.Logger
}

type traktConfig struct {
	appconfig.Trakt
	accessToken string
	username    string
}

func NewTraktClient(conf appconfig.Trakt, logger *slog.Logger) (TraktClientInterface, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	return &TraktClient{
		client: &http.Client{
			Jar: jar,
		},
		config: traktConfig{
			Trakt: conf,
		},
		logger: logger,
	}, nil
}

func (tc *TraktClient) Hydrate() error {
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
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseBrowser,
		Endpoint: traktPathAuthSignIn,
		Body:     http.NoBody,
	})
	if err != nil {
		return nil, err
	}
	return scrapeSelectionAttribute(response.Body, clientNameTrakt, "#new_user > input[name=authenticity_token]", "value")
}

func (tc *TraktClient) SignIn(authenticityToken string) error {
	data := url.Values{}
	data.Set(traktFormKeyAuthenticityToken, authenticityToken)
	data.Set(traktFormKeyUserLogIn, *tc.config.Email)
	data.Set(traktFormKeyUserPassword, *tc.config.Password)
	data.Set(traktFormKeyUserRemember, "1")
	encodedData := data.Encode()
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseBrowser,
		Endpoint: traktPathAuthSignIn,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	})
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}

func (tc *TraktClient) BrowseActivate() (*string, error) {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseBrowser,
		Endpoint: traktPathActivate,
		Body:     http.NoBody,
	})
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
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseBrowser,
		Endpoint: traktPathActivate,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	})
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
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseBrowser,
		Endpoint: traktPathActivateAuthorize,
		Body:     strings.NewReader(encodedData),
		Headers: map[string]string{
			traktHeaderKeyContentType:   "application/x-www-form-urlencoded",
			traktHeaderKeyContentLength: strconv.Itoa(len(encodedData)),
		},
	})
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
		ClientID:     *tc.config.ClientID,
		ClientSecret: *tc.config.ClientSecret,
	})
	if err != nil {
		return nil, err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathAuthTokens,
		Body:     bytes.NewReader(body),
		Headers: map[string]string{
			traktHeaderKeyContentType: "application/json",
		},
	})
	if err != nil {
		return nil, err
	}
	return readAuthTokensResponse(response.Body)
}

func (tc *TraktClient) GetAuthCodes() (*entities.TraktAuthCodesResponse, error) {
	body, err := json.Marshal(entities.TraktAuthCodesBody{ClientID: *tc.config.ClientID})
	if err != nil {
		return nil, err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathAuthCodes,
		Body:     bytes.NewReader(body),
		Headers: map[string]string{
			traktHeaderKeyApiVersion:  "2",
			traktHeaderKeyContentType: "application/json",
			traktHeaderKeyApiKey:      *tc.config.ClientID,
		},
	})
	if err != nil {
		return nil, err
	}
	return readAuthCodesResponse(response.Body)
}

func (tc *TraktClient) defaultApiHeaders() map[string]string {
	return map[string]string{
		traktHeaderKeyApiVersion:    "2",
		traktHeaderKeyContentType:   "application/json",
		traktHeaderKeyApiKey:        *tc.config.ClientID,
		traktHeaderKeyAuthorization: fmt.Sprintf("Bearer %s", tc.config.accessToken),
	}
}

func (tc *TraktClient) doRequest(requestFields requestFields) (*http.Response, error) {
	request, err := http.NewRequest(requestFields.Method, requestFields.BasePath+requestFields.Endpoint, ReusableReader(requestFields.Body))
	if err != nil {
		return nil, fmt.Errorf("error creating http request %s %s: %w", requestFields.Method, requestFields.BasePath+requestFields.Endpoint, err)
	}
	for key, value := range requestFields.Headers {
		request.Header.Set(key, value)
	}
	for retries := 0; retries < 5; retries++ {
		response, err := tc.client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("error sending http request %s, %s: %w", request.Method, request.URL, err)
		}
		switch response.StatusCode {
		case http.StatusOK, http.StatusCreated, http.StatusNoContent, http.StatusNotFound:
			return response, nil
		case traktStatusCodeEnhanceYourCalm:
			response.Body.Close()
			return nil, &ApiError{
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
			duration := time.Duration(retryAfter) * time.Second
			message := fmt.Sprintf("trakt rate limit reached, waiting for %s then retrying http request %s %s", duration, response.Request.Method, response.Request.URL)
			tc.logger.Warn(message)
			time.Sleep(duration)
			continue
		default:
			response.Body.Close()
			return nil, &ApiError{
				httpMethod: response.Request.Method,
				url:        response.Request.URL.String(),
				StatusCode: response.StatusCode,
				details:    fmt.Sprintf("unexpected status code %d", response.StatusCode),
			}
		}
	}
	return nil, fmt.Errorf("reached max retry attempts for %s %s", request.Method, request.URL)
}

func (tc *TraktClient) WatchlistGet() (*entities.TraktList, error) {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathWatchlist,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return nil, err
	}
	list := entities.TraktList{
		IDMeta: entities.TraktIDMeta{
			Slug: "watchlist",
		},
		IsWatchlist: true,
	}
	return readTraktListResponse(response.Body, list)
}

func (tc *TraktClient) WatchlistItemsAdd(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathWatchlist,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", slog.Any("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) WatchlistItemsRemove(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathWatchlistRemove,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt watchlist", slog.Any("watchlist", traktResponse))
	return nil
}

func (tc *TraktClient) ListGet(listID string) (*entities.TraktList, error) {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathUserListItems, tc.config.username, listID),
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, &ApiError{
			httpMethod: response.Request.Method,
			url:        response.Request.URL.String(),
			StatusCode: response.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", listID),
		}
	}
	list := entities.TraktList{
		IDMeta: entities.TraktIDMeta{
			Slug: listID,
		},
	}
	return readTraktListResponse(response.Body, list)
}

func (tc *TraktClient) ListItemsAdd(listID string, items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathUserListItems, tc.config.username, listID),
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", slog.Any(listID, traktResponse))
	return nil
}

func (tc *TraktClient) ListItemsRemove(listID string, items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathUserListItemsRemove, tc.config.username, listID),
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt list", slog.Any(listID, traktResponse))
	return nil
}

func (tc *TraktClient) ListsGet(idsMeta []entities.TraktIDMeta) ([]entities.TraktList, error) {
	var (
		outChan  = make(chan entities.TraktList, len(idsMeta))
		errChan  = make(chan error, 1)
		doneChan = make(chan struct{})
		lists    = make([]entities.TraktList, 0, len(idsMeta))
	)
	go func() {
		waitGroup := new(sync.WaitGroup)
		for _, idMeta := range idsMeta {
			waitGroup.Add(1)
			go func(idMeta entities.TraktIDMeta) {
				defer waitGroup.Done()
				list, err := tc.ListGet(idMeta.Slug)
				if err != nil {
					var apiError *ApiError
					if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
						if err = tc.ListAdd(idMeta.Slug, *idMeta.ListName); err != nil {
							errChan <- fmt.Errorf("failure creating trakt list %s: %w", idMeta.Slug, err)
						}
						outChan <- entities.TraktList{
							IDMeta: idMeta,
						}
						return
					}
					errChan <- fmt.Errorf("unexpected error while fetching trakt lists: %w", err)
					return
				}
				list.IDMeta = idMeta
				outChan <- *list
			}(idMeta)
		}
		waitGroup.Wait()
		close(doneChan)
	}()
	for {
		select {
		case list := <-outChan:
			lists = append(lists, list)
		case err := <-errChan:
			return nil, err
		case <-doneChan:
			return lists, nil
		}
	}
}

func (tc *TraktClient) ListAdd(listID, listName string) error {
	// TODO: let the user know that the list would have been created
	//if tc.config.SyncMode == appconfig.SyncModeDryRun {
	//	tc.logger.Info(fmt.Sprintf("sync mode dry run would have created trakt list %s", listID))
	//	return nil
	//}
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
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathUserList, tc.config.username, ""),
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	response.Body.Close()
	tc.logger.Info(fmt.Sprintf("created trakt list %s", listID))
	return nil
}

func (tc *TraktClient) ListRemove(listID string) error {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodDelete,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathUserList, tc.config.username, listID),
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	response.Body.Close()
	tc.logger.Info(fmt.Sprintf("removed trakt list %s", listID))
	return nil
}

func (tc *TraktClient) RatingsGet() (entities.TraktItems, error) {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathRatings,
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return nil, err
	}
	return readTraktItems(response.Body)
}

func (tc *TraktClient) RatingsAdd(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathRatings,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", slog.Any("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) RatingsRemove(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathRatingsRemove,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt ratings", slog.Any("ratings", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryGet(itemType, itemID string) (entities.TraktItems, error) {
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: traktPathBaseAPI,
		Endpoint: fmt.Sprintf(traktPathHistoryGet, itemType+"s", itemID, "1000"),
		Body:     http.NoBody,
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return nil, err
	}
	return readTraktItems(response.Body)
}

func (tc *TraktClient) HistoryAdd(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathHistory,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt history", slog.Any("history", traktResponse))
	return nil
}

func (tc *TraktClient) HistoryRemove(items entities.TraktItems) error {
	body, err := json.Marshal(mapTraktItemsToTraktBody(items))
	if err != nil {
		return err
	}
	response, err := tc.doRequest(requestFields{
		Method:   http.MethodPost,
		BasePath: traktPathBaseAPI,
		Endpoint: traktPathHistoryRemove,
		Body:     bytes.NewReader(body),
		Headers:  tc.defaultApiHeaders(),
	})
	if err != nil {
		return err
	}
	traktResponse, err := readTraktResponse(response.Body)
	if err != nil {
		return err
	}
	tc.logger.Info("synced trakt history", slog.Any("history", traktResponse))
	return nil
}

func mapTraktItemsToTraktBody(items entities.TraktItems) entities.TraktListBody {
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
	response := entities.TraktAuthCodesResponse{}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt auth codes response: %w", err)
	}
	return &response, nil
}

func readAuthTokensResponse(body io.ReadCloser) (*entities.TraktAuthTokensResponse, error) {
	defer body.Close()
	response := entities.TraktAuthTokensResponse{}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt auth tokens response: %w", err)
	}
	return &response, nil
}

func readTraktItems(body io.ReadCloser) (entities.TraktItems, error) {
	defer body.Close()
	var items entities.TraktItems
	if err := json.NewDecoder(body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt list: %w", err)
	}
	return items, nil
}

func readTraktListResponse(body io.ReadCloser, list entities.TraktList) (*entities.TraktList, error) {
	defer body.Close()
	if err := json.NewDecoder(body).Decode(&list.ListItems); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt list: %w", err)
	}
	return &list, nil
}

func readTraktResponse(body io.ReadCloser) (*entities.TraktResponse, error) {
	defer body.Close()
	response := entities.TraktResponse{}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt response: %w", err)
	}
	return &response, nil
}
