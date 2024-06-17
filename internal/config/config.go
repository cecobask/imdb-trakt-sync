package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type IMDb struct {
	CookieAtMain   *string  `koanf:"COOKIEATMAIN"`
	CookieUbidMain *string  `koanf:"COOKIEUBIDMAIN"`
	Lists          []string `koanf:"LISTS"`
}

type Trakt struct {
	Email        *string `koanf:"EMAIL"`
	Password     *string `koanf:"PASSWORD"`
	ClientID     *string `koanf:"CLIENTID"`
	ClientSecret *string `koanf:"CLIENTSECRET"`
}

type Sync struct {
	Mode        *string `koanf:"MODE"`
	SkipHistory *bool   `koanf:"SKIPHISTORY"`
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

	SyncModeAddOnly = "add-only"
	SyncModeDryRun  = "dry-run"
	SyncModeFull    = "full"
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
	if c.IMDb.CookieAtMain == nil {
		return fmt.Errorf("config field 'IMDB_COOKIEATMAIN' is required")
	}
	if c.IMDb.CookieUbidMain == nil {
		return fmt.Errorf("config field 'IMDB_COOKIEUBIDMAIN' is required")
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
	if c.Sync.SkipHistory == nil {
		return fmt.Errorf("config field 'SYNC_SKIPHISTORY' is required")
	}
	if !slices.Contains(validSyncModes(), *c.Sync.Mode) {
		return fmt.Errorf("config field 'SYNC_MODE' must be one of: %s", strings.Join(validSyncModes(), ", "))
	}
	c.stripSpace()
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

func (c *Config) stripSpace() {
	cookieAtMain := stripSpace(*c.IMDb.CookieAtMain)
	cookieUbidMain := stripSpace(*c.IMDb.CookieUbidMain)
	traktEmail := stripSpace(*c.Trakt.Email)
	traktClientID := stripSpace(*c.Trakt.ClientID)
	traktClientSecret := stripSpace(*c.Trakt.ClientSecret)
	syncMode := stripSpace(*c.Sync.Mode)
	c.IMDb.CookieAtMain = &cookieAtMain
	c.IMDb.CookieUbidMain = &cookieUbidMain
	c.Trakt.Email = &traktEmail
	c.Trakt.ClientID = &traktClientID
	c.Trakt.ClientSecret = &traktClientSecret
	c.Sync.Mode = &syncMode
}

func stripSpace(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func validSyncModes() []string {
	return []string{
		SyncModeFull,
		SyncModeAddOnly,
		SyncModeDryRun,
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
