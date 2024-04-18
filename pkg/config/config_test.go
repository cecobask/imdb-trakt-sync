package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	dummyConfig := `---
IMDB:
  COOKIEATMAIN: xXx
  COOKIEUBIDMAIN: xXx
  LISTS:
    - ls000000000
    - ls111111111
TRAKT:
  EMAIL: user@domain.com
  PASSWORD: password
  CLIENTID: xXx
  CLIENTSECRET: xXx
SYNC:
  MODE: dry-run
  SKIPHISTORY: true
`
	type args struct {
		includeEnv bool
	}
	tests := []struct {
		name         string
		args         args
		requirements func(*testing.T, string)
		assertions   func(*assert.Assertions, *Config, error)
	}{
		{
			name: "success creating config excluding env vars",
			args: args{
				includeEnv: false,
			},
			requirements: func(t *testing.T, path string) {
				err := os.WriteFile(path, []byte(dummyConfig), 0644)
				require.Nil(t, err)
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.Nil(err)
				assertions.NotNil(config)
				assertions.NotEmpty(config.IMDb.CookieAtMain)
				assertions.NotEmpty(config.IMDb.CookieUbidMain)
				assertions.Len(config.IMDb.Lists, 2)
				assertions.NotEmpty(config.Trakt.Email)
				assertions.NotEmpty(config.Trakt.Password)
				assertions.NotEmpty(config.Trakt.ClientID)
				assertions.NotEmpty(config.Trakt.ClientSecret)
				assertions.NotEmpty(config.Sync.Mode)
				assertions.NotEmpty(config.Sync.SkipHistory)
			},
		},
		{
			name: "success creating config including env vars",
			args: args{
				includeEnv: true,
			},
			requirements: func(t *testing.T, path string) {
				err := os.WriteFile(path, []byte(dummyConfig), 0644)
				require.Nil(t, err)
				t.Setenv("ITS_IMDB_COOKIEATMAIN", "test")
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.Nil(err)
				assertions.NotNil(config)
				assertions.NotNil(config.IMDb.CookieAtMain)
				assertions.Equal("test", *config.IMDb.CookieAtMain)
				assertions.NotEmpty(config.IMDb.CookieUbidMain)
				assertions.NotEmpty(config.IMDb.Lists)
				assertions.NotEmpty(config.Trakt.Email)
				assertions.NotEmpty(config.Trakt.Password)
				assertions.NotEmpty(config.Trakt.ClientID)
				assertions.NotEmpty(config.Trakt.ClientSecret)
				assertions.NotEmpty(config.Sync.Mode)
				assertions.NotEmpty(config.Sync.SkipHistory)
			},
		},
		{
			name: "invalid config file path",
			args: args{
				includeEnv: true,
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.NotNil(err)
				assertions.Nil(config)
			},
		},
		{
			name: "invalid config marshalling",
			args: args{
				includeEnv: true,
			},
			requirements: func(t *testing.T, path string) {
				err := os.WriteFile(path, []byte(dummyConfig), 0644)
				require.Nil(t, err)
				t.Setenv("ITS_SYNC_SKIPHISTORY", "invalid")
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.NotNil(err)
				assertions.Nil(config)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fmt.Sprintf("%s/config.yaml", t.TempDir())
			if tt.requirements != nil {
				tt.requirements(t, path)
			}
			config, err := New(path, tt.args.includeEnv)
			tt.assertions(assert.New(t), config, err)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	type fields struct {
		IMDb  IMDb
		Trakt Trakt
		Sync  Sync
	}
	tests := []struct {
		name       string
		fields     fields
		assertions func(*assert.Assertions, error)
	}{
		{
			name: "success",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:        new(string),
					Password:     new(string),
					ClientID:     new(string),
					ClientSecret: new(string),
				},
				Sync: Sync{
					Mode: func() *string {
						s := SyncModeFull
						return &s
					}(),
					SkipHistory: new(bool),
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Nil(err)
			},
		},
		{
			name: "missing IMDb.CookieAtMain",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "IMDB_COOKIEATMAIN")
			},
		},
		{
			name: "missing IMDb.CookieUbidMain",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "IMDB_COOKIEUBIDMAIN")
			},
		},
		{
			name: "missing Trakt.Email",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "TRAKT_EMAIL")
			},
		},
		{
			name: "missing Trakt.Password",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:    new(string),
					Password: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "TRAKT_PASSWORD")
			},
		},
		{
			name: "missing Trakt.ClientID",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:    new(string),
					Password: new(string),
					ClientID: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "TRAKT_CLIENTID")
			},
		},
		{
			name: "missing Trakt.ClientSecret",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:        new(string),
					Password:     new(string),
					ClientID:     new(string),
					ClientSecret: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "TRAKT_CLIENTSECRET")
			},
		},
		{
			name: "missing Sync.Mode",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:        new(string),
					Password:     new(string),
					ClientID:     new(string),
					ClientSecret: new(string),
				},
				Sync: Sync{
					Mode: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "SYNC_MODE")
			},
		},
		{
			name: "missing Sync.SkipHistory",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:        new(string),
					Password:     new(string),
					ClientID:     new(string),
					ClientSecret: new(string),
				},
				Sync: Sync{
					Mode: func() *string {
						s := SyncModeFull
						return &s
					}(),
					SkipHistory: nil,
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "SYNC_SKIPHISTORY")
			},
		},
		{
			name: "invalid Sync.Mode",
			fields: fields{
				IMDb: IMDb{
					CookieAtMain:   new(string),
					CookieUbidMain: new(string),
				},
				Trakt: Trakt{
					Email:        new(string),
					Password:     new(string),
					ClientID:     new(string),
					ClientSecret: new(string),
				},
				Sync: Sync{
					Mode: func() *string {
						s := "invalid"
						return &s
					}(),
					SkipHistory: new(bool),
				},
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.NotNil(err)
				assertions.Contains(err.Error(), "SYNC_MODE")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				IMDb:  tt.fields.IMDb,
				Trakt: tt.fields.Trakt,
				Sync:  tt.fields.Sync,
			}
			tt.assertions(assert.New(t), c.Validate())
		})
	}
}

func Test_environmentVariableModifier(t *testing.T) {
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, string, any)
	}{
		{
			name: "empty value",
			args: args{
				key:   "key",
				value: "",
			},
			assertions: func(assertions *assert.Assertions, key string, value any) {
				assertions.Equal("key", key)
				assertions.Nil(value)
			},
		},
		{
			name: "single value",
			args: args{
				key:   "key",
				value: "value1",
			},
			assertions: func(assertions *assert.Assertions, key string, value any) {
				assertions.Equal("key", key)
				assertions.Equal("value1", value)
			},
		},
		{
			name: "multiple values",
			args: args{
				key:   "key",
				value: "value1,value2",
			},
			assertions: func(assertions *assert.Assertions, key string, value any) {
				assertions.Equal("key", key)
				assertions.Equal([]string{
					"value1",
					"value2",
				}, value)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value := environmentVariableModifier(tt.args.key, tt.args.value)
			tt.assertions(assert.New(t), key, value)
		})
	}
}

func TestConfig_WriteFile(t *testing.T) {
	type fields struct {
		koanf *koanf.Koanf
	}
	type args struct {
		path string
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		assertions func(*assert.Assertions, error)
	}{
		{
			name: "success",
			fields: fields{
				koanf: koanf.New("."),
			},
			args: args{
				path: "config.yaml",
			},
			assertions: func(assertions *assert.Assertions, err error) {
				assertions.Nil(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				koanf: tt.fields.koanf,
			}
			tt.args.path = fmt.Sprintf("%s/%s", t.TempDir(), tt.args.path)
			tt.assertions(assert.New(t), c.WriteFile(tt.args.path))
		})
	}
}

func TestNewFromMap(t *testing.T) {
	type args struct {
		data map[string]interface{}
	}
	tests := []struct {
		name       string
		args       args
		want       *Config
		assertions func(*assert.Assertions, *Config, error)
	}{
		{
			name: "success",
			args: args{
				data: map[string]interface{}{
					"IMDB": map[string]interface{}{
						"COOKIEATMAIN": "xXx",
					},
				},
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.Nil(err)
				assertions.NotNil(config)
				assertions.NotNil(config.IMDb.CookieAtMain)
				assertions.Equal("xXx", *config.IMDb.CookieAtMain)
			},
		},
		{
			name: "invalid config",
			args: args{
				data: map[string]interface{}{
					"SYNC": map[string]interface{}{
						"SKIPHISTORY": "invalid",
					},
				},
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.NotNil(err)
				assertions.Nil(config)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf, err := NewFromMap(tt.args.data)
			tt.assertions(assert.New(t), conf, err)
		})
	}
}
