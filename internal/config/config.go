package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type IMDb struct {
	Auth           *string   `koanf:"AUTH"`
	Email          *string   `koanf:"EMAIL"`
	Password       *string   `koanf:"PASSWORD"`
	CookieAtMain   *string   `koanf:"COOKIEATMAIN"`
	CookieUbidMain *string   `koanf:"COOKIEUBIDMAIN"`
	Lists          *[]string `koanf:"LISTS"`
	Trace          *bool     `koanf:"TRACE"`
	Headless       *bool     `koanf:"HEADLESS"`
}

type Trakt struct {
	Email        *string `koanf:"EMAIL"`
	Password     *string `koanf:"PASSWORD"`
	ClientID     *string `koanf:"CLIENTID"`
	ClientSecret *string `koanf:"CLIENTSECRET"`
}

type Sync struct {
	Mode    *string        `koanf:"MODE"`
	History *bool          `koanf:"HISTORY"`
	Timeout *time.Duration `koanf:"TIMEOUT"`
}

type Config struct {
	koanf *koanf.Koanf
	IMDb  IMDb  `koanf:"IMDB"`
	Trakt Trakt `koanf:"TRAKT"`
	Sync  Sync  `koanf:"SYNC"`
}

const (
	delimiter = "_"
	prefix    = "ITS" + delimiter

	IMDbAuthMethodCredentials = "credentials"
	IMDbAuthMethodCookies     = "cookies"
	SyncModeAddOnly           = "add-only"
	SyncModeDryRun            = "dry-run"
	SyncModeFull              = "full"
	SyncTimeoutDefault        = time.Minute * 10
)

func New(path string, includeEnv bool) (*Config, error) {
	k := koanf.New(delimiter)
	fileProvider := file.Provider(path)
	if err := k.Load(fileProvider, yaml.Parser()); err != nil {
		return nil, fmt.Errorf("error loading config from yaml file: %w", err)
	}
	if includeEnv {
		envProvider := env.ProviderWithValue(prefix, delimiter, environmentVariableModifier)
		if err := k.Load(envProvider, nil); err != nil {
			return nil, fmt.Errorf("error loading config from environment variables: %w", err)
		}
	}
	conf := Config{
		koanf: k,
	}
	if err := k.Unmarshal("", &conf); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}
	conf.applyDefaults()
	return &conf, nil
}

func NewFromMap(data map[string]interface{}) (*Config, error) {
	k := koanf.New(delimiter)
	cmProvider := confmap.Provider(data, delimiter)
	if err := k.Load(cmProvider, nil); err != nil {
		return nil, err
	}
	conf := Config{
		koanf: k,
	}
	if err := k.Unmarshal("", &conf); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}
	return &conf, nil
}

func (c *Config) Validate() error {
	if c.IMDb.Auth == nil {
		return fmt.Errorf("config field 'IMDB_AUTH' is required")
	}
	switch *c.IMDb.Auth {
	case IMDbAuthMethodCredentials:
		if c.IMDb.Email == nil {
			return fmt.Errorf("config field 'IMDB_EMAIL' is required")
		}
		if c.IMDb.Password == nil {
			return fmt.Errorf("config field 'IMDB_PASSWORD' is required")
		}
	case IMDbAuthMethodCookies:
		if c.IMDb.CookieAtMain == nil {
			return fmt.Errorf("config field 'IMDB_COOKIEATMAIN' is required")
		}
		if c.IMDb.CookieUbidMain == nil {
			return fmt.Errorf("config field 'IMDB_COOKIEUBIDMAIN' is required")
		}
	default:
		return fmt.Errorf("config field 'IMDB_AUTH' must be one of: %s", strings.Join(validIMDbAuthMethods(), ", "))
	}
	if c.Trakt.Email == nil {
		return fmt.Errorf("config field 'TRAKT_EMAIL' is required")
	}
	if c.Trakt.Password == nil {
		return fmt.Errorf("config field 'TRAKT_PASSWORD' is required")
	}
	if c.Trakt.ClientID == nil {
		return fmt.Errorf("config field 'TRAKT_CLIENTID' is required")
	}
	if c.Trakt.ClientSecret == nil {
		return fmt.Errorf("config field 'TRAKT_CLIENTSECRET' is required")
	}
	if c.Sync.Mode == nil {
		return fmt.Errorf("config field 'SYNC_MODE' is required")
	}
	if !slices.Contains(validSyncModes(), *c.Sync.Mode) {
		return fmt.Errorf("config field 'SYNC_MODE' must be one of: %s", strings.Join(validSyncModes(), ", "))
	}
	return c.checkDummies()
}

func (c *Config) WriteFile(path string) error {
	data, err := c.koanf.Marshal(yaml.Parser())
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) Flatten() map[string]interface{} {
	return c.koanf.All()
}

func (c *Config) checkDummies() error {
	for k, v := range c.koanf.All() {
		if value, ok := v.(string); ok {
			if match := slices.Contains(dummyValues(), value); match {
				return fmt.Errorf("config field '%s' contains dummy value '%s'", k, value)
			}
			continue
		}
		if value, ok := v.([]interface{}); ok {
			for _, sliceElement := range value {
				if str, isStr := sliceElement.(string); isStr {
					if match := slices.Contains(dummyValues(), str); match {
						return fmt.Errorf("config field '%s' contains dummy value '%s'", k, str)
					}
				}
			}
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.IMDb.Auth == nil {
		c.IMDb.Auth = pointer(IMDbAuthMethodCredentials)
	}
	if c.IMDb.Lists == nil {
		c.IMDb.Lists = pointer(make([]string, 0))
	}
	if c.IMDb.Trace == nil {
		c.IMDb.Trace = pointer(false)
	}
	if c.IMDb.Headless == nil {
		c.IMDb.Headless = pointer(true)
	}
	if c.Sync.Mode == nil {
		c.Sync.Mode = pointer(SyncModeFull)
	}
	if c.Sync.History == nil {
		c.Sync.History = pointer(true)
	}
	if c.Sync.Timeout == nil {
		c.Sync.Timeout = pointer(SyncTimeoutDefault)
	}
}

func pointer[T any](v T) *T {
	return &v
}

func validSyncModes() []string {
	return []string{
		SyncModeFull,
		SyncModeAddOnly,
		SyncModeDryRun,
	}
}

func validIMDbAuthMethods() []string {
	return []string{
		IMDbAuthMethodCredentials,
		IMDbAuthMethodCookies,
	}
}

func dummyValues() []string {
	return []string{
		"user@domain.com",
		"password123",
		"zAta|RHiA67JIrBDPaswIym3GyrTlEuQH-u9yrKP3BUNCHgVyE4oNtUzBYVKlhjjzBiM_Z-GSVnH9rKW3Hf7LdbejovoF6SI4ZmgJcTIUXoA4NVcH1Qahwm0KYCyz95o1gsgby-uQwdU6CoS6MFTnjMkLe1puNiv4uFkvo8mOQulJJeutzYedxiUd0ns9w1X_WeVXPTZWjwisPZMw3EOR6-q9xR4kCEWRW7CmWxU1AEDQbT8ns_AJJD34w1nIQUkuLgBQrvJI_pY",
		"301-0710501-5367639",
		"ls000000000",
		"ls111111111",
		"828832482dea6fffa4453f849fe873de8be54791b9acc01f6923098d0a62972d",
		"bdf9bab88c17f3710a6394607e96cd3a21dee6e5ea0e0236e9ed06e425ed8b6f",
	}
}

func environmentVariableModifier(key string, value string) (string, any) {
	key = strings.TrimPrefix(key, prefix)
	if value == "" {
		switch key {
		case "IMDB_LISTS":
			return key, make([]string, 0)
		}
		return key, nil
	}
	if strings.Contains(value, ",") {
		return key, strings.Split(value, ",")
	}
	return key, value
}
