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

	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestImdbClient_doRequest(t *testing.T) {
	type args struct {
		requestFields requestFields
	}
	dummyRequestFields := requestFields{
		Method:   http.MethodGet,
		Endpoint: "/",
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
			c := &ImdbClient{
				client: http.DefaultClient,
			}
			res, err := c.doRequest(tt.args.requestFields)
			tt.assertions(assert.New(t), res, err)
		})
	}
}

func TestImdbClient_ListGet(t *testing.T) {
	type args struct {
		listId string
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *entities.ImdbList, error)
	}{
		{
			name: "successfully get list",
			args: args{
				listId: "ls123456789",
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
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListId)
				assertions.Equal("Watched (2023)", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(false, list.IsWatchlist)
				assertions.Equal("watched-2023", list.TraktListSlug)
			},
		},
		{
			name: "handle error when list is not found",
			args: args{
				listId: "ls123456789",
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
		{
			name: "handle unexpected status",
			args: args{
				listId: "ls123456789",
			},
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal("/list/ls123456789/export", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
				},
			}
			list, err := c.ListGet(tt.args.listId)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestImdbClient_WatchlistGet(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *entities.ImdbList, error)
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
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListId)
				assertions.Equal("WATCHLIST", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(true, list.IsWatchlist)
				assertions.Equal("watchlist", list.TraktListSlug)
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
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath:    testServer.URL,
					WatchlistId: "ls123456789",
				},
			}
			list, err := c.WatchlistGet()
			tt.assertions(assert.New(t), list, err)
		})
	}
}

func TestImdbClient_ListsGetAll(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.ImdbList, error)
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(2, len(lists))
				sort.Slice(lists, func(a, b int) bool {
					return lists[a].ListId < lists[b].ListId
				})
				assertions.Equal("ls123456789", lists[0].ListId)
				assertions.Equal("ls987654321", lists[1].ListId)
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
				assertions.NotNil(lists)
				assertions.Equal(0, len(lists))
				assertions.NoError(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
					UserId:   "ur12345678",
				},
				logger: logger.NewLogger(io.Discard),
			}
			lists, err := c.ListsGetAll()
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestImdbClient_ListsGet(t *testing.T) {
	type args struct {
		listIds []string
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.ImdbList, error)
	}{
		{
			name: "successfully get lists",
			args: args{
				listIds: []string{
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(2, len(lists))
				sort.Slice(lists, func(a, b int) bool {
					return lists[a].ListId < lists[b].ListId
				})
				assertions.Equal("ls123456789", lists[0].ListId)
				assertions.Equal("ls987654321", lists[1].ListId)
			},
		},
		{
			name: "silently ignore lists that could not be found",
			args: args{
				listIds: []string{
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
				assertions.NotNil(lists)
				assertions.NoError(err)
				assertions.Equal(0, len(lists))
			},
		},
		{
			name: "handle unexpected error when getting lists",
			args: args{
				listIds: []string{
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
			assertions: func(assertions *assert.Assertions, lists []entities.ImdbList, err error) {
				assertions.Nil(lists)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
					UserId:   "ur12345678",
				},
				logger: logger.NewLogger(io.Discard),
			}
			lists, err := c.ListsGet(tt.args.listIds)
			tt.assertions(assert.New(t), lists, err)
		})
	}
}

func TestImdbClient_UserIdScrape(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *ImdbClient, error)
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
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.NotNil(c)
				assertions.NoError(err)
				assertions.Equal("ur12345678", c.config.UserId)
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
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.Zero(c.config.UserId)
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
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.Zero(c.config.UserId)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
				},
			}
			err := c.UserIdScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestImdbClient_WatchlistIdScrape(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *ImdbClient, error)
	}{
		{
			name: "successfully scrape watchlist id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<meta property="pageId" content="ls123456789">`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.NotNil(c)
				assertions.NoError(err)
				assertions.Equal("ls123456789", c.config.WatchlistId)
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
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.Zero(c.config.WatchlistId)
				assertions.Error(err)
			},
		},
		{
			name: "fail to scrape watchlist id",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathWatchlist, r.URL.Path)
					w.WriteHeader(http.StatusOK)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *ImdbClient, err error) {
				assertions.Zero(c.config.WatchlistId)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
				},
			}
			err := c.WatchlistIdScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestImdbClient_RatingsGet(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, []entities.ImdbItem, error)
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
			assertions: func(assertions *assert.Assertions, ratings []entities.ImdbItem, err error) {
				assertions.NotNil(ratings)
				assertions.NoError(err)
				assertions.Equal(3, len(ratings))
				assertions.Equal("tt5013056", ratings[0].Id)
				assertions.Equal("tt15398776", ratings[1].Id)
				assertions.Equal("tt0172495", ratings[2].Id)
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
			assertions: func(assertions *assert.Assertions, ratings []entities.ImdbItem, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
					UserId:   "ur12345678",
				},
			}
			ratings, err := c.RatingsGet()
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

//go:embed testdata/imdb_list.csv
var dummyImdbList string

func Test_readImdbListResponse(t *testing.T) {
	type args struct {
		response *http.Response
		listId   string
	}

	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, *entities.ImdbList, error)
	}{
		{
			name: "successfully read list response",
			args: args{
				response: &http.Response{
					Header: http.Header{
						imdbHeaderKeyContentDisposition: []string{`attachment; filename="Watched (2023).csv"`},
					},
					Body: io.NopCloser(strings.NewReader(dummyImdbList)),
				},
				listId: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.NotNil(list)
				assertions.NoError(err)
				assertions.Equal("ls123456789", list.ListId)
				assertions.Equal("Watched (2023)", list.ListName)
				assertions.Equal(3, len(list.ListItems))
				assertions.Equal(false, list.IsWatchlist)
				assertions.Equal("watched-2023", list.TraktListSlug)
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
				listId: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
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
				listId: "ls123456789",
			},
			assertions: func(assertions *assert.Assertions, list *entities.ImdbList, err error) {
				assertions.Nil(list)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := readImdbListResponse(tt.args.response, tt.args.listId)
			tt.assertions(assert.New(t), list, err)
		})
	}
}

//go:embed testdata/imdb_ratings.csv
var dummyImdbRatings string

func Test_readImdbRatingsResponse(t *testing.T) {
	type args struct {
		response *http.Response
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, []entities.ImdbItem, error)
	}{
		{
			name: "successfully read ratings response",
			args: args{
				response: &http.Response{
					Body: io.NopCloser(strings.NewReader(dummyImdbRatings)),
				},
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.ImdbItem, err error) {
				assertions.NotNil(ratings)
				assertions.NoError(err)
				assertions.Equal(3, len(ratings))
				assertions.Equal("tt5013056", ratings[0].Id)
				assertions.Equal("tt15398776", ratings[1].Id)
				assertions.Equal("tt0172495", ratings[2].Id)
			},
		},
		{
			name: "handle error when parsing rating value",
			args: args{
				response: &http.Response{
					Body: io.NopCloser(strings.NewReader("field1,field2\n1,invalid-rating-value")),
				},
			},
			assertions: func(assertions *assert.Assertions, ratings []entities.ImdbItem, err error) {
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
			assertions: func(assertions *assert.Assertions, ratings []entities.ImdbItem, err error) {
				assertions.Nil(ratings)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings, err := readImdbRatingsResponse(tt.args.response)
			tt.assertions(assert.New(t), ratings, err)
		})
	}
}

func TestImdbClient_hydrate(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, ImdbConfig, error)
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
					bytes, err := w.Write([]byte(`<meta property="pageId" content="ls123456789">`))
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
			assertions: func(assertions *assert.Assertions, config ImdbConfig, err error) {
				assertions.NotNil(config)
				assertions.NoError(err)
				assertions.Equal("ur12345678", config.UserId)
				assertions.Equal("ls123456789", config.WatchlistId)
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
			assertions: func(assertions *assert.Assertions, config ImdbConfig, err error) {
				assertions.Zero(config.UserId)
				assertions.Zero(config.WatchlistId)
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
			assertions: func(assertions *assert.Assertions, config ImdbConfig, err error) {
				assertions.Equal("ur12345678", config.UserId)
				assertions.Zero(config.WatchlistId)
				assertions.Error(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := tt.requirements(require.New(t))
			defer testServer.Close()
			c := &ImdbClient{
				client: http.DefaultClient,
				config: ImdbConfig{
					BasePath: testServer.URL,
				},
			}
			tt.assertions(assert.New(t), c.config, c.hydrate())
		})
	}
}
