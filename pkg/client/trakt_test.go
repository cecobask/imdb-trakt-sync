package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
)

func buildTestTraktClient(config traktConfig) TraktClientInterface {
	return &TraktClient{
		client: &http.Client{
			Transport: httpmock.DefaultTransport,
		},
		config: config,
		logger: logger.NewLogger(io.Discard),
	}
}

func stringPointer(s string) *string {
	return &s
}

var (
	dummyUsername          = "cecobask"
	dummyListID            = "watched"
	dummyListName          = "Watched"
	dummyItemID            = "1388"
	dummyAuthenticityToken = "authenticity-token-value"
	dummyUserCode          = "0e887e88"
	dummyDeviceCode        = "4eca8122d271cf8a17f96b00326d2e83c8e699ee8cb836f9d812aa71cb535b6b"
	dummyAppConfigTrakt    = appconfig.Trakt{
		Email:        stringPointer(""),
		Password:     stringPointer(""),
		ClientID:     stringPointer(""),
		ClientSecret: stringPointer(""),
	}
	dummyConfig = traktConfig{
		Trakt:    dummyAppConfigTrakt,
		username: dummyUsername,
	}
	dummyIDsMeta = []entities.TraktIDMeta{
		{
			IMDb: "ls123456789",
			Slug: dummyListID,
			ListName: func() *string {
				listName := dummyListName
				return &listName
			}(),
		},
		{
			IMDb: "ls987654321",
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
				IDMeta: entities.TraktIDMeta{
					IMDb: "tt5013056",
				},
			},
		},
		{
			Type: entities.TraktItemTypeShow,
			Show: entities.TraktItemSpec{
				IDMeta: entities.TraktIDMeta{
					IMDb: "tt0903747",
				},
			},
		},
		{
			Type: entities.TraktItemTypeEpisode,
			Show: entities.TraktItemSpec{
				IDMeta: entities.TraktIDMeta{
					IMDb: "tt0959621",
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
			name: "handle retryable status",
			args: args{
				requestFields: dummyRequestFields,
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(dummyRequestFields.Method, r.Method)
					requirements.Equal(dummyRequestFields.Endpoint, r.URL.Path)
					w.WriteHeader(http.StatusBadGateway)
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
				assertions.Equal("watchlist", list.IDMeta.Slug)
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader into target")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(dummyConfig)
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
		config traktConfig
	}
	tests := []struct {
		name         string
		args         args
		fields       fields
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully add watchlist items",
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
					httpmock.NewJsonResponderOrPanic(http.StatusCreated, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure adding watchlist items",
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
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
		config traktConfig
	}
	tests := []struct {
		name         string
		args         args
		fields       fields
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully remove watchlist items",
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
					httpmock.NewJsonResponderOrPanic(http.StatusNoContent, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NoError(err)
			},
		},
		{
			name: "failure removing watchlist items",
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.WatchlistItemsRemove(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListGet(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		listID string
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
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal(dummyListID, list.IDMeta.Slug)
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
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
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
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewJsonResponderOrPanic(http.StatusNotFound, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				var notFoundErr *TraktListNotFoundError
				assertions.True(errors.As(err, &notFoundErr))
				assertions.Equal(dummyListID, notFoundErr.Slug)
				assertions.Equal(notFoundErr.Error(), fmt.Sprintf("list with id %s could not be found", dummyListID))
			},
		},
		{
			name: "failure decoding trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewStringResponder(http.StatusOK, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, list *entities.TraktList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure decoding reader into target")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			list, err := c.ListGet(tt.args.listID)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestTraktClient_ListItemsAdd(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		listID string
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
			name: "successfully add list items",
			fields: fields{
				config: traktConfig{
					Trakt:    dummyAppConfigTrakt,
					username: dummyUsername,
				},
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
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
				config: traktConfig{
					Trakt:    dummyAppConfigTrakt,
					username: dummyUsername,
				},
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
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
			name: "failure decoding trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewStringResponder(http.StatusCreated, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.ListItemsAdd(tt.args.listID, tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListItemsRemove(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		listID string
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
			name: "successfully remove list items",
			fields: fields{
				config: traktConfig{
					Trakt:    dummyAppConfigTrakt,
					username: dummyUsername,
				},
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListID),
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
				config: traktConfig{
					Trakt:    dummyAppConfigTrakt,
					username: dummyUsername,
				},
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListID),
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
			name: "failure decoding trakt response",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listID: dummyListID,
				items:  dummyItems,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodPost,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItemsRemove, dummyUsername, dummyListID),
					httpmock.NewStringResponder(http.StatusNoContent, "invalid"),
				)
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Error(err)
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.ListItemsRemove(tt.args.listID, tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListsGet(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		idsMeta []entities.TraktIDMeta
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, []entities.TraktList, []error)
	}{
		{
			name: "successfully get lists",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIDsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, errs []error) {
				assertions.NotNil(lists)
				assertions.Empty(errs)
				validSlugs := []string{
					dummyListID,
					"not-watched",
				}
				for _, list := range lists {
					isMatch := slices.Contains(validSlugs, list.IDMeta.Slug)
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
				idsMeta: dummyIDsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusInternalServerError, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, errs []error) {
				assertions.Nil(lists)
				assertions.Len(errs, 1)
				var apiError *ApiError
				assertions.True(errors.As(errs[0], &apiError))
				assertions.Equal(http.StatusInternalServerError, apiError.StatusCode)
			},
		},
		{
			name: "delegate empty trakt list error when not found",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				idsMeta: dummyIDsMeta,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, dummyListID),
					httpmock.NewJsonResponderOrPanic(http.StatusOK, httpmock.File("testdata/trakt_list.json")),
				)
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserListItems, dummyUsername, "not-watched"),
					httpmock.NewJsonResponderOrPanic(http.StatusNotFound, nil),
				)
			},
			assertions: func(assertions *assert.Assertions, lists []entities.TraktList, errs []error) {
				assertions.NotNil(lists)
				assertions.Len(errs, 1)
				var notFoundErr *TraktListNotFoundError
				assertions.True(errors.As(errs[0], &notFoundErr))
				assertions.Equal("not-watched", notFoundErr.Slug)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			lists, err := c.ListsGet(tt.args.idsMeta)
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestTraktClient_ListAdd(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		listID   string
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
				listID:   dummyListID,
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
				listID:   dummyListID,
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.ListAdd(tt.args.listID, tt.args.listName)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_ListRemove(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		listID string
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		requirements func()
		assertions   func(*assert.Assertions, error)
	}{
		{
			name: "successfully remove list",
			fields: fields{
				config: dummyConfig,
			},
			args: args{
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodDelete,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, dummyListID),
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
				listID: dummyListID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodDelete,
					fmt.Sprintf(traktPathBaseAPI+traktPathUserList, dummyUsername, dummyListID),
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
			c := buildTestTraktClient(tt.fields.config)
			err := c.ListRemove(tt.args.listID)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_RatingsGet(t *testing.T) {
	type fields struct {
		config traktConfig
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			ratings, err := c.RatingsGet()
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

func TestTraktClient_RatingsAdd(t *testing.T) {
	type fields struct {
		config traktConfig
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.RatingsAdd(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_RatingsRemove(t *testing.T) {
	type fields struct {
		config traktConfig
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.RatingsRemove(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_HistoryGet(t *testing.T) {
	type fields struct {
		config traktConfig
	}
	type args struct {
		itemType string
		itemID   string
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
				itemID:   dummyItemID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathHistoryGet, entities.TraktItemTypeShow+"s", dummyItemID, "1000"),
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
				itemID:   dummyItemID,
			},
			requirements: func() {
				httpmock.RegisterResponder(
					http.MethodGet,
					fmt.Sprintf(traktPathBaseAPI+traktPathHistoryGet, entities.TraktItemTypeShow+"s", dummyItemID, "1000"),
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
			c := buildTestTraktClient(tt.fields.config)
			history, err := c.HistoryGet(tt.args.itemType, tt.args.itemID)
			tt.assertions(assert.New(t), history, err)
		})
	}
}

func TestTraktClient_HistoryAdd(t *testing.T) {
	type fields struct {
		config traktConfig
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
			err := c.HistoryAdd(tt.args.items)
			tt.assertions(assert.New(t), err)
		})
	}
}

func TestTraktClient_HistoryRemove(t *testing.T) {
	type fields struct {
		config traktConfig
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
			name: "failure decoding trakt response",
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
				assertions.Contains(err.Error(), "failure decoding reader")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()
			tt.requirements()
			c := buildTestTraktClient(tt.fields.config)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
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
			c := buildTestTraktClient(dummyConfig)
			err := c.Hydrate()
			tt.assertions(assert.New(t), err)
		})
	}
}
