package client

import (
	_ "embed"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
)

func populateHttpResponseWithFileContents(w http.ResponseWriter, filename string) error {
	f, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	_, err = w.Write(f)
	if err != nil {
		return err
	}
	return nil
}

func TestIMDbClient_doRequest(t *testing.T) {
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
			name: "failure creating http request",
			args: args{
				requestFields: requestFields{
					Method: "Ø",
				},
			},
			requirements: func(assertions *require.Assertions) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			},
			assertions: func(assertions *assert.Assertions, res *http.Response, err error) {
				assertions.Nil(res)
				assertions.Error(err)
				assertions.ErrorContains(err, "failure creating http request")
			},
		},
		{
			name: "handle unexpected status",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			tt.args.requestFields.BasePath = testServer.URL
			c := &IMDbClient{
				client: http.DefaultClient,
			}
			res, err := c.doRequest(tt.args.requestFields)
			tt.assertions(assert.New(t), res, err)
		})
	}
}

func TestIMDbClient_ListGet(t *testing.T) {
	type args struct {
		listID string
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *entities.IMDbList, error)
	}{
		{
			name: "successfully get list",
			args: args{
				listID: "ls123456789",
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.Header().Set(imdbHeaderKeyContentDisposition, `attachment; filename="Watched (2023).csv"`)
					w.WriteHeader(http.StatusOK)
					requirements.NoError(populateHttpResponseWithFileContents(w, "testdata/imdb_list.csv"))
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListID)
				assertions.Equal("Watched (2023)", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(false, list.IsWatchlist)
			},
		},
		{
			name: "handle error when list is not found",
			args: args{
				listID: "ls123456789",
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
		{
			name: "handle unexpected status",
			args: args{
				listID: "ls123456789",
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
				},
			}
			list, err := c.ListGet(tt.args.listID)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestIMDbClient_WatchlistGet(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *entities.IMDbList, error)
	}{
		{
			name: "successfully get watchlist",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.Header().Set(imdbHeaderKeyContentDisposition, `attachment; filename="WATCHLIST.csv"`)
					w.WriteHeader(http.StatusOK)
					requirements.NoError(populateHttpResponseWithFileContents(w, "testdata/imdb_list.csv"))
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListID)
				assertions.Equal("WATCHLIST", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(true, list.IsWatchlist)
			},
		},
		{
			name: "fail to get watchlist",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath:    testServer.URL,
					watchlistID: "ls123456789",
				},
			}
			list, err := c.WatchlistGet()
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestIMDbClient_ListsGetAll(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.IMDbList, error)
	}{
		{
			name: "successfully get all lists",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					allowedPaths := map[string]bool{
						"/user/ur12345678/lists":   true,
						"/list/ls123456789/export": true,
						"/list/ls987654321/export": true,
					}
					requirements.Equal(http.MethodGet, r.Method)
					requirements.True(allowedPaths[r.URL.Path])
					filename := "testdata/imdb_list.csv"
					if r.URL.Path == "/user/ur12345678/lists" {
						filename = "testdata/imdb_lists.html"
					}
					w.Header().Set(imdbHeaderKeyContentDisposition, `attachment; filename="DummyList.csv"`)
					w.WriteHeader(http.StatusOK)
					requirements.NoError(populateHttpResponseWithFileContents(w, filename))
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(2, len(lists))
				sort.Slice(lists, func(a, b int) bool {
					return lists[a].ListID < lists[b].ListID
				})
				assertions.Equal("ls123456789", lists[0].ListID)
				assertions.Equal("ls987654321", lists[1].ListID)
			},
		},
		{
			name: "fail to get all lists",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/user/ur12345678/lists", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
			},
		},
		{
			name: "fail to find lists in html response",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/user/ur12345678/lists", r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<ul><li class="user-list"></li></ul>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
					userID:   "ur12345678",
				},
				logger: logger.NewLogger(io.Discard),
			}
			lists, err := c.ListsGetAll()
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestIMDbClient_ListsGet(t *testing.T) {
	type args struct {
		listIDs []string
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.IMDbList, error)
	}{
		{
			name: "successfully get lists",
			args: args{
				listIDs: []string{
					"ls123456789",
					"ls987654321",
				},
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					allowedPaths := map[string]bool{
						"/list/ls123456789/export": true,
						"/list/ls987654321/export": true,
					}
					requirements.Equal(http.MethodGet, r.Method)
					requirements.True(allowedPaths[r.URL.Path])
					w.Header().Set(imdbHeaderKeyContentDisposition, `attachment; filename="DummyList.csv"`)
					w.WriteHeader(http.StatusOK)
					requirements.NoError(populateHttpResponseWithFileContents(w, "testdata/imdb_list.csv"))
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(2, len(lists))
				sort.Slice(lists, func(a, b int) bool {
					return lists[a].ListID < lists[b].ListID
				})
				assertions.Equal("ls123456789", lists[0].ListID)
				assertions.Equal("ls987654321", lists[1].ListID)
			},
		},
		{
			name: "silently ignore lists that could not be found",
			args: args{
				listIDs: []string{
					"ls123456789",
				},
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(0, len(lists))
			},
		},
		{
			name: "handle unexpected error when getting lists",
			args: args{
				listIDs: []string{
					"ls123456789",
				},
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, lists []entities.IMDbList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
					userID:   "ur12345678",
				},
				logger: logger.NewLogger(io.Discard),
			}
			lists, err := c.ListsGet(tt.args.listIDs)
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestIMDbClient_UserIDScrape(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *IMDbClient, error)
	}{
		{
			name: "successfully scrape user id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathProfile, r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<div class="user-profile userId" data-userid="ur12345678"></div>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.NotNil(c)
				assertions.NoError(err)
				assertions.Equal("ur12345678", c.config.userID)
			},
		},
		{
			name: "handle unexpected status",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathProfile, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.Zero(c.config.userID)
				assertions.Error(err)
			},
		},
		{
			name: "fail to scrape user id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathProfile, r.URL.Path)
					w.WriteHeader(http.StatusOK)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.Zero(c.config.userID)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
				},
			}
			err := c.UserIDScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestIMDbClient_WatchlistIDScrape(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *IMDbClient, error)
	}{
		{
			name: "successfully scrape watchlist id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<a data-testid="hero-list-subnav-edit-button" href="/list/ls123456789/edit">Edit</a>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.NotNil(c)
				assertions.NoError(err)
				assertions.Equal("ls123456789", c.config.watchlistID)
			},
		},
		{
			name: "handle unexpected status",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.Zero(c.config.watchlistID)
				assertions.Error(err)
			},
		},
		{
			name: "fail to scrape watchlist href",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusOK)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.Zero(c.config.watchlistID)
				assertions.Error(err)
			},
		},
		{
			name: "fail to scrape watchlist id from invalid href",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<a data-testid="hero-list-subnav-edit-button" href="/list">Edit</a>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.Zero(c.config.watchlistID)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
				},
			}
			err := c.WatchlistIDScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestIMDbClient_RatingsGet(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.IMDbItem, error)
	}{
		{
			name: "successfully get ratings",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/user/ur12345678/ratings/export", r.URL.Path)
					w.WriteHeader(http.StatusOK)
					requirements.NoError(populateHttpResponseWithFileContents(w, "testdata/imdb_ratings.csv"))
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.IMDbItem, err error) {
				assertions.NotNil(ratings)
				assertions.NoError(err)
				assertions.Equal(3, len(ratings))
				assertions.Equal("tt5013056", ratings[0].ID)
				assertions.Equal("tt15398776", ratings[1].ID)
				assertions.Equal("tt0172495", ratings[2].ID)
			},
		},
		{
			name: "handle unexpected status",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/user/ur12345678/ratings/export", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.IMDbItem, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
					userID:   "ur12345678",
				},
			}
			ratings, err := c.RatingsGet()
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

//go:embed testdata/imdb_list.csv
var dummyIMDbList string

func Test_readIMDbListResponse(t *testing.T) {
	type args struct {
		response *http.Response
		listID   string
	}

	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, *entities.IMDbList, error)
	}{
		{
			name: "successfully read list response",
			args: args{
				response: &http.Response{
					Header: http.Header{
						imdbHeaderKeyContentDisposition: []string{`attachment; filename="Watched (2023).csv"`},
					},
					Body: io.NopCloser(strings.NewReader(dummyIMDbList)),
				},
				listID: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListID)
				assertions.Equal("Watched (2023)", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(false, list.IsWatchlist)
			},
		},
		{
			name: "handle error when parsing media type",
			args: args{
				response: &http.Response{
					Header: http.Header{
						imdbHeaderKeyContentDisposition: []string{`invalid media type`},
					},
					Body: http.NoBody,
				},
				listID: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
		{
			name: "handle error when content disposition header is missing",
			args: args{
				response: &http.Response{
					Body: http.NoBody,
				},
				listID: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.IMDbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := readIMDbListResponse(tt.args.response, tt.args.listID)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

//go:embed testdata/imdb_ratings.csv
var dummyIMDbRatings string

func Test_readIMDbRatingsResponse(t *testing.T) {
	type args struct {
		response *http.Response
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, []entities.IMDbItem, error)
	}{
		{
			name: "successfully read ratings response",
			args: args{
				response: &http.Response{
					Body: io.NopCloser(strings.NewReader(dummyIMDbRatings)),
				},
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.IMDbItem, err error) {
				assertions.NotNil(ratings)
				assertions.NoError(err)
				assertions.Equal(3, len(ratings))
				assertions.Equal("tt5013056", ratings[0].ID)
				assertions.Equal("tt15398776", ratings[1].ID)
				assertions.Equal("tt0172495", ratings[2].ID)
			},
		},
		{
			name: "handle error when parsing rating value",
			args: args{
				response: &http.Response{
					Body: io.NopCloser(strings.NewReader("field1,field2\n1,invalid-rating-value")),
				},
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.IMDbItem, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
			},
		},
		{
			name: "handle error when parsing rating date",
			args: args{
				response: &http.Response{
					Body: io.NopCloser(strings.NewReader("field1,field2,field3\n1,1,invalid-date")),
				},
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.IMDbItem, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings, err := readIMDbRatingsResponse(tt.args.response)
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

func TestIMDbClient_Hydrate(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, imdbConfig, error)
	}{
		{
			name: "successfully hydrate client",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				profileHandler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<div class="user-profile userId" data-userid="ur12345678"></div>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				watchlistHandler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<a data-testid="hero-list-subnav-edit-button" href="/list/ls123456789/edit">Edit</a>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case imdbPathProfile:
						profileHandler(w, r)
					case imdbPathWatchlist:
						watchlistHandler(w, r)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			},
			assertions: func(assertions *assert.Assertions, config imdbConfig, err error) {
				assertions.NotNil(config)
				assertions.NoError(err)
				assertions.Equal("ur12345678", config.userID)
				assertions.Equal("ls123456789", config.watchlistID)
			},
		},
		{
			name: "handle error when scraping user id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathProfile, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			assertions: func(assertions *assert.Assertions, config imdbConfig, err error) {
				assertions.Zero(config.userID)
				assertions.Zero(config.watchlistID)
				assertions.Error(err)
			},
		},
		{
			name: "handle error when scraping watchlist id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case imdbPathProfile:
						requirements.Equal(http.MethodGet, r.Method)
						w.WriteHeader(http.StatusOK)
						bytes, err := w.Write([]byte(`<div class="user-profile userId" data-userid="ur12345678"></div>`))
						requirements.Greater(bytes, 0)
						requirements.NoError(err)
					case imdbPathWatchlist:
						requirements.Equal(http.MethodGet, r.Method)
						w.WriteHeader(http.StatusInternalServerError)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			},
			assertions: func(assertions *assert.Assertions, config imdbConfig, err error) {
				assertions.Equal("ur12345678", config.userID)
				assertions.Zero(config.watchlistID)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &IMDbClient{
				client: http.DefaultClient,
				config: imdbConfig{
					basePath: testServer.URL,
				},
			}
			tt.assertions(assert.New(t), c.config, c.Hydrate())
		})
	}
}

func TestNewIMDbClient(t *testing.T) {
	type args struct {
		config appconfig.IMDb
	}
	dummyIMDbConfig := appconfig.IMDb{
		CookieAtMain:   stringPointer(""),
		CookieUbidMain: stringPointer(""),
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, IMDbClientInterface, error)
	}{
		{
			name: "successfully create client",
			args: args{
				config: dummyIMDbConfig,
			},
			assertions: func(assertions *assert.Assertions, client IMDbClientInterface, err error) {
				assertions.NotNil(client)
				imdbClient, ok := client.(*IMDbClient)
				assertions.True(ok)
				assertions.Equal("https://www.imdb.com", imdbClient.config.basePath)
				assertions.NoError(err)
			},
		},
		{
			name: "successfully create client with default base path",
			args: args{
				config: dummyIMDbConfig,
			},
			assertions: func(assertions *assert.Assertions, client IMDbClientInterface, err error) {
				assertions.NotNil(client)
				imdbClient, ok := client.(*IMDbClient)
				assertions.True(ok)
				assertions.Equal(imdbPathBase, imdbClient.config.basePath)
				assertions.NoError(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewIMDbClient(tt.args.config, logger.NewLogger(io.Discard))
			tt.assertions(assert.New(t), client, err)
		})
	}
}
