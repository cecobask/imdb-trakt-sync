package config

import (
	"fmt"
	"os"
	"regexp"
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
	Mode      *string        `koanf:"MODE"`
	History   *bool          `koanf:"HISTORY"`
	Ratings   *bool          `koanf:"RATINGS"`
	Watchlist *bool          `koanf:"WATCHLIST"`
	Timeout   *time.Duration `koanf:"TIMEOUT"`
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
	IMDbAuthMethodNone        = "none"
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
	conf.applyDefaults()
	return &conf, nil
}

func (c *Config) Validate() error {
	if isNilOrEmpty(c.IMDb.Auth) {
		return fmt.Errorf("field 'IMDB_AUTH' is required")
	}
	switch *c.IMDb.Auth {
	case IMDbAuthMethodCredentials:
		if isNilOrEmpty(c.IMDb.Email) {
			return fmt.Errorf("field 'IMDB_EMAIL' is required")
		}
		if isNilOrEmpty(c.IMDb.Password) {
			return fmt.Errorf("field 'IMDB_PASSWORD' is required")
		}
	case IMDbAuthMethodCookies:
		if isNilOrEmpty(c.IMDb.CookieAtMain) {
			return fmt.Errorf("field 'IMDB_COOKIEATMAIN' is required")
		}
		if isNilOrEmpty(c.IMDb.CookieUbidMain) {
			return fmt.Errorf("field 'IMDB_COOKIEUBIDMAIN' is required")
		}
	case IMDbAuthMethodNone:
	default:
		return fmt.Errorf("field 'IMDB_AUTH' must be one of: %s", strings.Join(validIMDbAuthMethods(), ", "))
	}
	if err := c.validateListIdentifiers(); err != nil {
		return fmt.Errorf("field 'IMDB_LISTS' is invalid: %w", err)
	}
	if isNilOrEmpty(c.Trakt.Email) {
		return fmt.Errorf("field 'TRAKT_EMAIL' is required")
	}
	if isNilOrEmpty(c.Trakt.Password) {
		return fmt.Errorf("field 'TRAKT_PASSWORD' is required")
	}
	if isNilOrEmpty(c.Trakt.ClientID) {
		return fmt.Errorf("field 'TRAKT_CLIENTID' is required")
	}
	if isNilOrEmpty(c.Trakt.ClientSecret) {
		return fmt.Errorf("field 'TRAKT_CLIENTSECRET' is required")
	}
	if isNilOrEmpty(c.Sync.Mode) {
		return fmt.Errorf("field 'SYNC_MODE' is required")
	}
	if !slices.Contains(validSyncModes(), *c.Sync.Mode) {
		return fmt.Errorf("field 'SYNC_MODE' must be one of: %s", strings.Join(validSyncModes(), ", "))
	}
	return c.checkDummies()
}

func (c *Config) validateListIdentifiers() error {
	re := regexp.MustCompile(`^ls[0-9]{9}$`)
	for _, id := range *c.IMDb.Lists {
		if ok := re.MatchString(id); !ok {
			return fmt.Errorf("valid list id starts with ls and is followed by 9 digits, but got %s", id)
		}
	}
	return nil
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
				return fmt.Errorf("field '%s' contains dummy value '%s'", k, value)
			}
			continue
		}
		if value, ok := v.([]interface{}); ok {
			for _, sliceElement := range value {
				if str, isStr := sliceElement.(string); isStr {
					if match := slices.Contains(dummyValues(), str); match {
						return fmt.Errorf("field '%s' contains dummy value '%s'", k, str)
					}
				}
			}
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.IMDb.Lists == nil {
		c.IMDb.Lists = pointer(make([]string, 0))
	}
	if c.IMDb.Trace == nil {
		c.IMDb.Trace = pointer(false)
	}
	if c.IMDb.Headless == nil {
		c.IMDb.Headless = pointer(true)
	}
	if c.Sync.History == nil {
		c.Sync.History = pointer(false)
	}
	if c.Sync.Ratings == nil {
		c.Sync.Ratings = pointer(false)
	}
	if c.Sync.Watchlist == nil {
		c.Sync.Watchlist = pointer(false)
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
		IMDbAuthMethodNone,
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
		return key, nil
	}
	if strings.Contains(value, ",") {
		return key, strings.Split(value, ",")
	}
	return key, value
}

func isNilOrEmpty(value *string) bool {
	return value == nil || *value == ""
}
