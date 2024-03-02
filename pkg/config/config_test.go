package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name         string
		args         args
		requirements func(t *testing.T)
		assertions   func(*assert.Assertions, *Config, error)
	}{
		{
			name: "success",
			args: args{
				path: "default.yaml",
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.Nil(err)
				assertions.NotNil(config)
				assertions.NotEmpty(config.IMDb.CookieAtMain)
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
			name: "invalid file path",
			args: args{
				path: "invalid.yaml",
			},
			assertions: func(assertions *assert.Assertions, config *Config, err error) {
				assertions.NotNil(err)
				assertions.Nil(config)
			},
		},
		{
			name: "invalid marshalling",
			args: args{
				path: "default.yaml",
			},
			requirements: func(t *testing.T) {
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
			if tt.requirements != nil {
				tt.requirements(t)
			}
			config, err := New(tt.args.path)
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
