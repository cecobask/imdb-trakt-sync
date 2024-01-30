package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestTraktClient(config TraktConfig) TraktClientInterface {
	return &TraktClient{
		client: &http.Client{
			Transport: httpmock.DefaultTransport,
		},
		config: config,
		logger: logger.NewLogger(io.Discard),
	}

}

var (
	dummyUsername          = "cecobask"
	dummyListId            = "watched"
	dummyListName          = "Watched"
	dummyItemId            = "1388"
	dummyAuthenticityToken = "authenticity-token-value"
	dummyUserCode          = "0e887e88"
	dummyDeviceCode        = "4eca8122d271cf8a17f96b00326d2e83c8e699ee8cb836f9d812aa71cb535b6b"
	dummyConfig            = TraktConfig{
		username: dummyUsername,
	}
	dummyIdsMeta = []entities.TraktIdMeta{
		{
			Imdb: "ls123456789",
			Slug: dummyListId,
			ListName: func() *string {
				listName := dummyListName
				return &listName
			}(),
		},
		{
			Imdb: "ls987654321",
			Slug: "not-watched",
			ListName: func() *string {
				listName := "Not Watched"
				return &listName
			}(),
		},
	}
	dummyItems = entities.TraktItems{
		{
			Type: entities.TraktItemTypeMovie,
			Movie: entities.TraktItemSpec{
				IdMeta: entities.TraktIdMeta{
					Imdb: "tt5013056",
				},
			},
		},
		{
			Type: entities.TraktItemTypeShow,
			Show: entities.TraktItemSpec{
				IdMeta: entities.TraktIdMeta{
					Imdb: "tt0903747",
				},
			},
		},
		{
			Type: entities.TraktItemTypeEpisode,
			Show: entities.TraktItemSpec{
				IdMeta: entities.TraktIdMeta{
					Imdb: "tt0959621",
				},
			},
		},
		{
			Type: entities.TraktItemTypeSeason,
		},
	}
	dummyRequestFields = requestFields{
		Method:   http.MethodPost,
		Endpoint: "/",
		Body:     http.NoBody,
		Headers: map[string]string{
			traktHeaderKeyContentType: "application/json",
		},
	}
)

func TestTraktClient_doRequest(t *testing.T) {
	type args struct {
		requestFields requestFields
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *http.Response, error)
	}{
		{
			name: "handle response delegation",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.WriteHeader(http.StatusOK)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.NotNil(res)
				assertions.NoError(err)
				assertions.Equal(http.StatusOK, res.StatusCode)
			},
		},
		{
			name: "handle status enhance your calm",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.WriteHeader(traktStatusCodeEnhanceYourCalm)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.Nil(res)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(traktStatusCodeEnhanceYourCalm, apiError.StatusCode)
			},
		},
		{
			name: "handle status too many requests",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.Header().Set(traktHeaderKeyRetryAfter, "0")
					w.WriteHeader(http.StatusTooManyRequests)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.Nil(res)
				assertions.Error(err)
				assertions.Contains(err.Error(), "reached max retry attempts")
			},
		},
		{
			name: "handle unexpected status code",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.Nil(res)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure parsing retry after header",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.Header().Set(traktHeaderKeyRetryAfter, "invalid")
					w.WriteHeader(http.StatusTooManyRequests)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.Nil(res)
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure parsing the value of trakt header")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			tt.args.requestFields.BasePath = testServer.URL
			c := &TraktClient{
				client: http.DefaultClient,
				logger: logger.NewLogger(io.Discard),
			}
			res, err := c.doRequest(tt.args.requestFields)
			tt.assertions(assert.New(t), res, err)
		})
	}
}

func TestTraktClient_WatchlistGet(t *testing.T) {
	tests := []struct {
		name         string
		requirements func()
		assertions   func(*assert.Assertions, *entities.TraktList, error)
	}{
		{
			name: "successfully get watchlist",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("watchlist", list.IdMeta.Slug)
				assertions.Equal(true, list.IsWatchlist)
				assertions.Equal(3, len(list.ListItems))
			},
		},
		{
			name: "failure getting watchlist",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt list",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewStringResponder(http.StatusOK, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt list")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(TraktConfig{
				SyncMode: validSyncModes()[0],
			})
			watchlist, err := c.WatchlistGet()
			tt.assertions(assert.New(t), watchlist, err)
		})
	}
}

func TestTraktClient_WatchlistItemsAdd(t *testing.T) {
	type args struct {
		items entities.TraktItems
	}
	type fields struct {
		config TraktConfig
	}
	tests := []struct {
		name         string
		args         args
		fields       fields
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			args: args{
				items: dummyItems,
			},
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully add watchlist items",
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewJsonResponderOrPanic(http.StatusCreated, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding watchlist items",
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlist,
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.WatchlistItemsAdd(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_WatchlistItemsRemove(t *testing.T) {
	type args struct {
		items entities.TraktItems
	}
	type fields struct {
		config TraktConfig
	}
	tests := []struct {
		name         string
		args         args
		fields       fields
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			args: args{
				items: dummyItems,
			},
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully remove watchlist items",
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlistRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing watchlist items",
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlistRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathWatchlistRemove,
					httpmock.NewStringResponder(http.StatusNoContent, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.WatchlistItemsRemove(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListGet(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		listId string
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, *entities.TraktList, error)
	}{
		{
			name: "successfully get list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal(dummyListId, list.IdMeta.Slug)
				assertions.Equal(false, list.IsWatchlist)
				assertions.Equal(3, len(list.ListItems))
			},
		},
		{
			name: "failure getting list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "handle not found error",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusNotFound, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusNotFound, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			list, err := c.ListGet(tt.args.listId)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestTraktClient_ListItemsAdd(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		listId string
		items  entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully add list items",
			fields: fields{
				config: TraktConfig{
					username: dummyUsername,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusCreated, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding list items",
			fields: fields{
				config: TraktConfig{
					username: dummyUsername,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.ListItemsAdd(tt.args.listId, tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListItemsRemove(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		listId string
		items  entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully remove list items",
			fields: fields{
				config: TraktConfig{
					username: dummyUsername,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing list items",
			fields: fields{
				config: TraktConfig{
					username: dummyUsername,
				},
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListId),
					httpmock.NewStringResponder(http.StatusNoContent, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.ListItemsRemove(tt.args.listId, tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListsGet(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		idsMeta []entities.TraktIdMeta
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, []entities.TraktList, error)
	}{
		{
			name: "successfully get lists",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIdsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				validSlugs := []string{
					dummyListId,
					"not-watched",
				}
				for _, list := range lists {
					isMatch := slices.Contains(validSlugs, list.IdMeta.Slug)
					assertions.Equal(true, isMatch)
				}
			},
		},
		{
			name: "failure getting lists",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIdsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "create empty trakt list when not found",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIdsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusNotFound, nil),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, ""),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				validSlugs := []string{
					dummyListId,
					"not-watched",
				}
				for _, list := range lists {
					isMatch := slices.Contains(validSlugs, list.IdMeta.Slug)
					assertions.Equal(true, isMatch)
				}
			},
		},
		{
			name: "failure creating empty trakt list when not found",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIdsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusNotFound, nil),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, ""),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			lists, err := c.ListsGet(tt.args.idsMeta)
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestTraktClient_ListAdd(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		listId   string
		listName string
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully add list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId:   dummyListId,
				listName: dummyListName,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, ""),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId:   dummyListId,
				listName: dummyListName,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, ""),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				listId:   dummyListId,
				listName: dummyListName,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.ListAdd(tt.args.listId, tt.args.listName)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListRemove(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		listId string
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully remove list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodDelete,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listId: dummyListId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodDelete,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, dummyListId),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.ListRemove(tt.args.listId)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_RatingsGet(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	tests := []struct {
		name         string
		fields       fields
		requirements func()
		assertions   func(*assert.Assertions, entities.TraktItems, error)
	}{
		{
			name: "successfully get ratings",
			fields: fields{
				config: dummyConfig,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_ratings.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, ratings entities.TraktItems, err error) {
				assertions.NotNil(ratings)
				assertions.NoError(err)
				assertions.Equal(3, len(ratings))
			},
		},
		{
			name: "failure getting ratings",
			fields: fields{
				config: dummyConfig,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, ratings entities.TraktItems, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt list",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewStringResponder(http.StatusOK, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, ratings entities.TraktItems, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt list")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			ratings, err := c.RatingsGet()
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

func TestTraktClient_RatingsAdd(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		items entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully add ratings",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewJsonResponderOrPanic(http.StatusCreated, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding ratings",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatings,
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.RatingsAdd(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_RatingsRemove(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		items entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully remove ratings",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatingsRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing ratings",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatingsRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathRatingsRemove,
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.RatingsRemove(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_HistoryGet(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		itemType string
		itemId   string
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, entities.TraktItems, error)
	}{
		{
			name: "successfully get history",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				itemType: entities.TraktItemTypeShow,
				itemId:   dummyItemId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathHistoryGet, entities.TraktItemTypeShow+"s", dummyItemId, "1000"),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_history.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, history entities.TraktItems, err error) {
				assertions.NotNil(history)
				assertions.NoError(err)
				assertions.Equal(1, len(history))
			},
		},
		{
			name: "failure getting history",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				itemType: entities.TraktItemTypeShow,
				itemId:   dummyItemId,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathHistoryGet, entities.TraktItemTypeShow+"s", dummyItemId, "1000"),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, history entities.TraktItems, err error) {
				assertions.Nil(history)
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			history, err := c.HistoryGet(tt.args.itemType, tt.args.itemId)
			tt.assertions(assert.New(t), history, err)
		})
	}
}

func TestTraktClient_HistoryAdd(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		items entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully add history items",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistory,
					httpmock.NewJsonResponderOrPanic(http.StatusCreated, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding history items",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistory,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistory,
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.HistoryAdd(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_HistoryRemove(t *testing.T) {
	type fields struct {
		config TraktConfig
	}
	type args struct {
		items entities.TraktItems
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully handle dry-run mode",
			fields: fields{
				config: TraktConfig{
					SyncMode: traktSyncModeDryRun,
				},
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "successfully remove history items",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistoryRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing history items",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistoryRemove,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure unmarshalling trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				items: dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathHistoryRemove,
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure unmarshalling trakt response")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(tt.fields.config)
			err := c.HistoryRemove(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_BrowseSignIn(t *testing.T) {
	tests := []struct {
		name         string
		requirements func()
		assertions   func(*assert.Assertions, *string, error)
	}{
		{
			name: "successfully browse sign in",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.NoError(err)
				assertions.NotNil(token)
				assertions.Equal(dummyAuthenticityToken, *token)
			},
		},
		{
			name: "failure getting authenticity token",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.Error(err)
				assertions.Nil(token)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			token, err := c.BrowseSignIn()
			tt.assertions(assert.New(t), token, err)
		})
	}
}

func TestTraktClient_SignIn(t *testing.T) {
	type args struct {
		authenticityToken string
	}
	tests := []struct {
		name         string
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully sign in",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure signing in",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			err := c.SignIn(tt.args.authenticityToken)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_BrowseActivate(t *testing.T) {
	tests := []struct {
		name         string
		requirements func()
		assertions   func(*assert.Assertions, *string, error)
	}{
		{
			name: "successfully browse activate",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><form class="form-signin"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div>`),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.NoError(err)
				assertions.NotNil(token)
				assertions.Equal(dummyAuthenticityToken, *token)
			},
		},
		{
			name: "failure browsing activate",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.Error(err)
				assertions.Nil(token)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			token, err := c.BrowseActivate()
			tt.assertions(assert.New(t), token, err)
		})
	}
}

func TestTraktClient_Activate(t *testing.T) {
	type args struct {
		userCode          string
		authenticityToken string
	}
	tests := []struct {
		name         string
		args         args
		requirements func()
		assertions   func(*assert.Assertions, *string, error)
	}{
		{
			name: "successfully activate",
			args: args{
				userCode:          dummyUserCode,
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><div class="form-signin less-top"><div><form><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div></div></div>`),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.NoError(err)
				assertions.NotNil(token)
				assertions.Equal(dummyAuthenticityToken, *token)
			},
		},
		{
			name: "failure activating",
			args: args{
				userCode:          dummyUserCode,
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, token *string, err error) {
				assertions.Error(err)
				assertions.Nil(token)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			token, err := c.Activate(tt.args.userCode, tt.args.authenticityToken)
			tt.assertions(assert.New(t), token, err)
		})
	}
}

func TestTraktClient_ActivateAuthorize(t *testing.T) {
	type args struct {
		authenticityToken string
	}
	tests := []struct {
		name         string
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully activate authorize",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewStringResponder(http.StatusOK, `<a id="desktop-user-avatar" href="/users/cecobask"></a>`),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure activating authorize",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "failure scraping username",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure scraping")
			},
		},
		{
			name: "failure parsing scrape result to username",
			args: args{
				authenticityToken: dummyAuthenticityToken,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewStringResponder(http.StatusOK, `<a id="desktop-user-avatar" href="invalid"></a>`),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure scraping")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			err := c.ActivateAuthorize(tt.args.authenticityToken)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_GetAccessToken(t *testing.T) {
	type args struct {
		deviceCode string
	}
	tests := []struct {
		name         string
		args         args
		requirements func()
		assertions   func(*assert.Assertions, *entities.TraktAuthTokensResponse, error)
	}{
		{
			name: "successfully get access token",
			args: args{
				deviceCode: dummyDeviceCode,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthTokens,
					httpmock.NewStringResponder(http.StatusOK, `{"access_token":"access-token-value"}`),
				)
			},
			assertions: func(assertions *assert.Assertions, response *entities.TraktAuthTokensResponse, err error) {
				assertions.NoError(err)
				assertions.NotNil(response)
				assertions.Equal("access-token-value", response.AccessToken)
			},
		},
		{
			name: "failure getting access token",
			args: args{
				deviceCode: dummyDeviceCode,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthTokens,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, response *entities.TraktAuthTokensResponse, err error) {
				assertions.Error(err)
				assertions.Nil(response)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			response, err := c.GetAccessToken(tt.args.deviceCode)
			tt.assertions(assert.New(t), response, err)
		})
	}
}

func TestTraktClient_GetAuthCodes(t *testing.T) {
	tests := []struct {
		name         string
		requirements func()
		assertions   func(*assert.Assertions, *entities.TraktAuthCodesResponse, error)
	}{
		{
			name: "successfully get auth codes",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
			},
			assertions: func(assertions *assert.Assertions, response *entities.TraktAuthCodesResponse, err error) {
				assertions.NoError(err)
				assertions.NotNil(response)
				assertions.Equal(dummyDeviceCode, response.DeviceCode)
				assertions.Equal(dummyUserCode, response.UserCode)
			},
		},
		{
			name: "failure getting auth codes",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, response *entities.TraktAuthCodesResponse, err error) {
				assertions.Error(err)
				assertions.Nil(response)
				var apiError *ApiError
				assertions.True(errors.As(err, &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			response, err := c.GetAuthCodes()
			tt.assertions(assert.New(t), response, err)
		})
	}
}

func TestTraktClient_Hydrate(t *testing.T) {
	tests := []struct {
		name         string
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully hydrate",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><form class="form-signin"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><div class="form-signin less-top"><div><form><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div></div></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewStringResponder(http.StatusOK, `<a id="desktop-user-avatar" href="/users/cecobask"></a>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthTokens,
					httpmock.NewStringResponder(http.StatusOK, `{"access_token":"access-token-value"}`),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure getting auth codes",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure generating auth codes")
			},
		},
		{
			name: "failure browsing sign in",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure simulating browse to the trakt sign in page")
			},
		},
		{
			name: "failure signing in",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure simulating trakt sign in form submission")
			},
		},
		{
			name: "failure browsing activate",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure simulating browse to the trakt device activation page")
			},
		},
		{
			name: "failure activating",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><form class="form-signin"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure simulating trakt device activation form submission")
			},
		},
		{
			name: "failure activating authorize",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><form class="form-signin"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><div class="form-signin less-top"><div><form><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div></div></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure simulating trakt api app allowlisting")
			},
		},
		{
			name: "failure getting access token",
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthCodes,
					httpmock.NewStringResponder(http.StatusOK, `{"device_code":"`+dummyDeviceCode+`","user_code":"`+dummyUserCode+`"}`),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewStringResponder(http.StatusOK, `<div id="new_user"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathAuthSignIn,
					httpmock.NewJsonResponderOrPanic(http.StatusOK, nil),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><form class="form-signin"><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivate,
					httpmock.NewStringResponder(http.StatusOK, `<div id="auth-form-wrapper"><div class="form-signin less-top"><div><form><input type="hidden" name="authenticity_token" value="authenticity-token-value"></form></div></div></div>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseBrowser+traktPathActivateAuthorize,
					httpmock.NewStringResponder(http.StatusOK, `<a id="desktop-user-avatar" href="/users/cecobask"></a>`),
				)
				httpmock.RegisterResponder(
					http.MethodPost,
					traktPathBaseAPI+traktPathAuthTokens,
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure exchanging trakt device code for access token")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := defaultTestTraktClient(dummyConfig)
			err := c.Hydrate()
			tt.assertions(assert.New(t), err)
		})
	}
}
