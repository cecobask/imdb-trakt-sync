package trakt

import (
	"bytes"
	"context"
	"encoding/json"
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
	pathUserList            = "/users/%s/lists/%d"
	pathUserListItems       = "/users/%s/lists/%d/items"
	pathUserListItemsRemove = "/users/%s/lists/%d/items/remove"
	pathUserLists           = "/users/%s/lists"
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
	ListAdd(ctx context.Context, name string) (*IDMeta, error)
	ListGet(ctx context.Context, lid int) (*List, error)
	ListGetMeta(ctx context.Context, lid int) (*List, error)
	ListItemsAdd(ctx context.Context, lid int, its Items) error
	ListItemsRemove(ctx context.Context, lid int, its Items) error
	ListsGet(ctx context.Context, ids IDMetas) (Lists, error)
	ListsGetAllMeta(ctx context.Context) (Lists, error)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathHistory, nil, body, nil, http.StatusCreated)
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
	path := fmt.Sprintf(pathHistoryGet, itType+"s", itID)
	query := map[string][]string{
		"limit": {"1000"},
	}
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, query, http.NoBody, nil, http.StatusOK, http.StatusNotFound)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathHistoryRemove, nil, body, nil, http.StatusOK)
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

func (c *client) ListAdd(ctx context.Context, name string) (*IDMeta, error) {
	b, err := json.Marshal(listAddBody{
		Name:        name,
		Description: fmt.Sprintf("List imported from IMDb using https://github.com/cecobask/imdb-trakt-sync on %v", time.Now().Format(time.RFC1123)),
	})
	if err != nil {
		return nil, fmt.Errorf("failure marshaling list add body: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserLists, c.username)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, nil, body, nil, http.StatusCreated)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	defer resp.Body.Close()
	var list List
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failure decoding list response: %w", err)
	}

	c.logger.Info("created trakt list", "name", name, "id", list.IDMeta.Trakt)
	return &list.IDMeta, nil
}

func (c *client) ListGet(ctx context.Context, lid int) (*List, error) {
	list, err := c.ListGetMeta(ctx, lid)
	if err != nil {
		return nil, fmt.Errorf("failure getting list meta: %w", err)
	}
	path := fmt.Sprintf(pathUserListItems, c.username, lid)
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, nil, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	litems, err := decodeJSON[Items](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding list response: %w", err)
	}
	list.ListItems = litems
	return list, nil
}

func (c *client) ListGetMeta(ctx context.Context, lid int) (*List, error) {
	path := fmt.Sprintf(pathUserList, c.username, lid)
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, nil, http.NoBody, nil, http.StatusOK, http.StatusNoContent)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, NewListNotFoundError(lid)
	}
	list, err := decodeJSON[List](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding list response: %w", err)
	}
	return &list, nil
}

func (c *client) ListItemsAdd(ctx context.Context, lid int, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling list items: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserListItems, c.username, lid)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, nil, body, nil, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	l, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding list items response: %w", err)
	}
	c.logger.Info("synced trakt list", slog.Any("items", l))
	return nil
}

func (c *client) ListItemsRemove(ctx context.Context, lid int, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return fmt.Errorf("failure marshaling list items: %w", err)
	}
	body := bytes.NewReader(b)
	path := fmt.Sprintf(pathUserListItemsRemove, c.username, lid)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, path, nil, body, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("failure doing request: %w", err)
	}
	l, err := decodeJSON[response](resp.Body)
	if err != nil {
		return fmt.Errorf("failure decoding list items response: %w", err)
	}
	c.logger.Info("synced trakt list", slog.Any("items", l))
	return nil
}

func (c *client) ListsGet(ctx context.Context, ids IDMetas) (Lists, error) {
	var (
		outChan  = make(chan List, len(ids))
		errChan  = make(chan error, 1)
		doneChan = make(chan struct{})
		lists    = make(Lists, 0, len(ids))
	)
	go func() {
		waitGroup := new(sync.WaitGroup)
		for _, id := range ids {
			waitGroup.Add(1)
			go func(id IDMeta) {
				defer waitGroup.Done()
				list, err := c.ListGet(ctx, id.Trakt)
				if err != nil {
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
			return nil, err
		case <-doneChan:
			return lists, nil
		}
	}
}

func (c *client) ListsGetAllMeta(ctx context.Context) (Lists, error) {
	path := fmt.Sprintf(pathUserLists, c.username)
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, path, nil, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("failure doing request: %w", err)
	}
	lists, err := decodeJSON[Lists](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding lists response: %w", err)
	}
	c.logger.Info("fetched existing trakt lists metadata", "count", len(lists))

	return lists, nil
}

func (c *client) RatingsAdd(ctx context.Context, its Items) error {
	b, err := json.Marshal(its.toListBody())
	if err != nil {
		return err
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathRatings, nil, body, nil, http.StatusCreated)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathRatings, nil, http.NoBody, nil, http.StatusOK)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathRatingsRemove, nil, body, nil, http.StatusOK)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathWatchlist, nil, http.NoBody, nil, http.StatusOK)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathWatchlist, nil, body, nil, http.StatusCreated)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodPost, c.baseURL, pathWatchlistRemove, nil, body, nil, http.StatusOK)
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
	resp, err := doRequest(ctx, c.httpClient, http.MethodGet, c.baseURL, pathUserInfo, nil, http.NoBody, nil, http.StatusOK)
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
