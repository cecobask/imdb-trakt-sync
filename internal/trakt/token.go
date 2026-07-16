package trakt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type storedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (t *storedToken) toAccessToken() *accessToken {
	return &accessToken{
		token:        t.AccessToken,
		refreshToken: t.RefreshToken,
		expiresAt:    t.ExpiresAt,
	}
}

// loadToken returns (nil, nil) when the token file does not exist yet, since
// that is the expected state before the first device authorization completes.
func loadToken(path string) (*storedToken, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failure reading trakt token file %s: %w", path, err)
	}
	var t storedToken
	if err = json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("failure unmarshalling trakt token file %s: %w", path, err)
	}
	return &t, nil
}

func saveToken(path string, t *storedToken) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("failure marshalling trakt token: %w", err)
	}
	if err = os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("failure writing trakt token file %s: %w", path, err)
	}
	return nil
}
