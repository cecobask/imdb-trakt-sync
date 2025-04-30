package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/cecobask/imdb-trakt-sync/internal/config"
)

const (
	pathAuthCodes           = "/oauth/device/code"
	pathAuthTokens          = "/oauth/device/token"
	pathBaseAPI             = "https://api.trakt.tv"
	pathHistory             = "/sync/history"
	pathHistoryGet          = "/sync/history/%s/%s"
	pathHistoryRemove       = "/sync/history/remove"
	pathRatings             = "/sync/ratings"
	pathRatingsRemove       = "/sync/ratings/remove"
	pathUserInfo            = "/users/me"
	pathUserLists           = "/users/%s/lists"
	pathUserListItems       = "/users/%s/lists/%s/items"
	pathUserListItemsRemove = "/users/%s/lists/%s/items/remove"
	pathWatchlist           = "/sync/watchlist"
	pathWatchlistRemove     = "/sync/watchlist/remove"
)

type client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	username   string
}

type API interface {
	HistoryAdd(ctx context.Context, its Items) error
	HistoryGet(ctx context.Context, itType, itID string) (Items, error)
	HistoryRemove(ctx context.Context, its Items) error
	ListAdd(ctx context.Context, slug, name string) error
	ListGet(ctx context.Context, slug string) (*List, error)
	ListItemsAdd(ctx context.Context, slug string, its Items) error
	ListItemsRemove(ctx context.Context, slug string, its Items) error
	ListsGet(ctx context.Context, ids IDMetas) (Lists, []error)
	RatingsAdd(ctx context.Context, its Items) error
	RatingsGet(ctx context.Context) (Items, error)
	RatingsRemove(ctx context.Context, its Items) error
	WatchlistGet(ctx context.Context) (*List, error)
	WatchlistItemsAdd(ctx context.Context, its Items) error
	WatchlistItemsRemove(ctx context.Context, its Items) error
}

func NewAPI(ctx context.Context, conf config.Trakt, logger *slog.Logger) (API, error) {
	retryTrans := newRetryTransport(http.DefaultTransport, logger)
	transport := newAuthTransport(
		retryTrans,
		newAuthClient(conf, retryTrans),
		NewBrowser(conf, retryTrans),
		*conf.ClientID,
	)
	c := &client{
		baseURL: pathBaseAPI,
		httpClient: &http.Client{
			Transport: transport,
		},
		logger: logger,
	}
	ui, err := c.getUserInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failure getting user info: %w", err)
	}
	c.username = ui.Username
	return c, nil
}

func (c *client) HistoryAdd(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return err
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathHistory, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	h, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding history response: %w", err)
	}
	c.logger.Info("synced trakt history", slog.Any("history", h))
	return nil
}

func (c *client) HistoryGet(ctx context.Context, itType, itID string) (Items, error) {
	path := fmt.Sprintf(pathHistoryGet+"?limit=1000", itType+"s", itID)
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, http.NoBody, nil, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	h, err := decodeJSON[Items](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding history response: %w", err)
	}
	return h, nil
}

func (c *client) HistoryRemove(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return err
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathHistoryRemove, body, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	defer resp.Body.Close()
	h, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding history response: %w", err)
	}
	c.logger.Info("synced trakt history", slog.Any("history", h))
	return nil
}

func (c *client) ListAdd(ctx context.Context, slug, name string) error {
	b, err := json.Marshal(listAddBody{
		Name:           name,
		Description:    fmt.Sprintf("List imported from IMDb using https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
		Privacy:        "public",
		DisplayNumbers: false,
		AllowComments:  true,
		SortBy:         "rank",
		SortHow:        "asc",
	})
	if err != nil {
		return fmt.Errorf("failure marshaling list add body: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserLists, c.username)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	defer resp.Body.Close()
	c.logger.Info("created trakt list " + slug)
	return nil
}

func (c *client) ListGet(ctx context.Context, slug string) (*List, error) {
	path := fmt.Sprintf(pathUserListItems, c.username, slug)
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, http.NoBody, nil, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, NewListNotFoundError(slug)
	}
	litems, err := decodeJSON[Items](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding list response: %w", err)
	}
	return &List{
		IDMeta: IDMeta{
			Slug: slug,
		},
		ListItems: litems,
	}, nil
}

func (c *client) ListItemsAdd(ctx context.Context, slug string, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling list items: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserListItems, c.username, slug)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	l, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding list items response: %w", err)
	}
	c.logger.Info("synced trakt list", slog.Any(slug, l))
	return nil
}

func (c *client) ListItemsRemove(ctx context.Context, slug string, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling list items: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserListItemsRemove, c.username, slug)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, body, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	l, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding list items response: %w", err)
	}
	c.logger.Info("synced trakt list", slog.Any(slug, l))
	return nil
}

func (c *client) ListsGet(ctx context.Context, ids IDMetas) (Lists, []error) {
	var (
		outChan         = make(chan List, len(ids))
		errChan         = make(chan error, 1)
		doneChan        = make(chan struct{})
		lists           = make(Lists, 0, len(ids))
		delegatedErrors = make([]error, 0, len(ids))
	)
	go func() {
		waitGroup := new(sync.WaitGroup)
		for _, id := range ids {
			waitGroup.Add(1)
			go func(id IDMeta) {
				defer waitGroup.Done()
				list, err := c.ListGet(ctx, id.Slug)
				if err != nil {
					var lnferr *ListNotFoundError
					if errors.As(err, &lnferr) {
						delegatedErrors = append(delegatedErrors, err)
						return
					}
					errChan <- fmt.Errorf("unexpected error while fetching lists: %w", err)
					return
				}
				list.IDMeta = id
				outChan <- *list
			}(id)
		}
		waitGroup.Wait()
		close(doneChan)
	}()
	for {
		select {
		case list := <-outChan:
			lists = append(lists, list)
		case err := <-errChan:
			return nil, []error{err}
		case <-doneChan:
			return lists, delegatedErrors
		}
	}
}

func (c *client) RatingsAdd(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return err
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathRatings, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	r, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding ratings response: %w", err)
	}
	c.logger.Info("synced trakt ratings", slog.Any("ratings", r))
	return nil
}

func (c *client) RatingsGet(ctx context.Context) (Items, error) {
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathRatings, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	r, err := decodeJSON[Items](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding ratings response: %w", err)
	}
	return r, nil
}

func (c *client) RatingsRemove(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling ratings items: %w", err)
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathRatingsRemove, body, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	r, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding ratings response: %w", err)
	}
	c.logger.Info("synced trakt ratings", slog.Any("ratings", r))
	return nil
}

func (c *client) WatchlistGet(ctx context.Context) (*List, error) {
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathWatchlist, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	witems, err := decodeJSON[Items](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding watchlist response: %w", err)
	}
	return &List{
		IDMeta: IDMeta{
			Slug: "watchlist",
		},
		ListItems:   witems,
		IsWatchlist: true,
	}, nil
}

func (c *client) WatchlistItemsAdd(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling watchlist items: %w", err)
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathWatchlist, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	w, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding watchlist response: %w", err)
	}
	c.logger.Info("synced trakt watchlist", slog.Any("watchlist", w))
	return nil
}

func (c *client) WatchlistItemsRemove(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling watchlist items: %w", err)
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathWatchlistRemove, body, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	w, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding watchlist response: %w", err)
	}
	c.logger.Info("synced trakt watchlist", slog.Any("watchlist", w))
	return nil
}

func (c *client) getUserInfo(ctx context.Context) (*userInfo, error) {
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathUserInfo, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	ui, err := decodeJSON[userInfo](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding user info response: %w", err)
	}
	return &ui, nil
}

func decodeJSON[T any](rc io.ReadCloser) (T, error) {
	defer rc.Close()
	var resp T
	if err := json.NewDecoder(rc).Decode(&resp); err != nil {
		return resp, fmt.Errorf("failure decoding json: %w", err)
	}
	return resp, nil
}
