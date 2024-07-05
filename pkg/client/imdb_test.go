package client

import (
	_ "embed"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestIMDbClient_userIdentityScrape(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *IMDbClient, error)
	}{
		{
			name: "successfully scrape user identity",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				handler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					requirements.Equal(imdbPathProfile, r.URL.Path)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<div class="user-profile userId" data-userid="ur12345678"></div><div class="header"><h1>testusername</h1></div>`))
					requirements.Greater(bytes, 0)
					requirements.NoError(err)
				}
				return httptest.NewServer(http.HandlerFunc(handler))
			},
			assertions: func(assertions *assert.Assertions, c *IMDbClient, err error) {
				assertions.NotNil(c)
				assertions.NoError(err)
				assertions.Equal("ur12345678", c.config.userID)
				assertions.Equal("testusername", c.config.username)
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
		{
			name: "fail to scrape username",
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
				assertions.Equal("ur12345678", c.config.userID)
				assertions.Zero(c.config.username)
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
				config: &imdbConfig{
					basePath: testServer.URL,
				},
			}
			err := c.userIdentityScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestIMDbClient_watchlistIDScrape(t *testing.T) {
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
				config: &imdbConfig{
					basePath: testServer.URL,
				},
			}
			err := c.watchlistIDScrape()
			tt.assertions(assert.New(t), c, err)
		})
	}
}

func TestIMDbClient_hydrate(t *testing.T) {
	tests := []struct {
		name         string
		requirements func(*require.Assertions) *httptest.Server
		assertions   func(*assert.Assertions, *imdbConfig, error)
	}{
		{
			name: "successfully hydrate client",
			requirements: func(requirements *require.Assertions) *httptest.Server {
				profileHandler := func(w http.ResponseWriter, r *http.Request) {
					requirements.Equal(http.MethodGet, r.Method)
					w.WriteHeader(http.StatusOK)
					bytes, err := w.Write([]byte(`<div class="user-profile userId" data-userid="ur12345678"></div><div class="header"><h1>testusername</h1></div>`))
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
			assertions: func(assertions *assert.Assertions, config *imdbConfig, err error) {
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
			assertions: func(assertions *assert.Assertions, config *imdbConfig, err error) {
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
			assertions: func(assertions *assert.Assertions, config *imdbConfig, err error) {
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
				config: &imdbConfig{
					basePath: testServer.URL,
				},
			}
			tt.assertions(assert.New(t), c.config, c.hydrate())
		})
	}
}
