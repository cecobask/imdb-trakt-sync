package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cecobask/imdb-trakt-sync/internal/config"
)

type authClient struct {
	baseURL string
	client  *http.Client
	conf    config.Trakt
}

type authCodesBody struct {
	ClientID string `json:"client_id"`
}

type authCodesResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type authTokensBody struct {
	Code         string `json:"code,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token,omitempty"`
	GrantType    string `json:"grant_type"`
}

type authTokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	CreatedAt    int    `json:"created_at"`
	ExpiresIn    int    `json:"expires_in"`
}

type authTransport struct {
	authClient    *authClient
	next          http.RoundTripper
	accessToken   *accessToken
	clientID      string
	tokenFilePath string
	logger        *slog.Logger
}

type accessToken struct {
	token        string
	refreshToken string
	expiresAt    time.Time
}

const (
	grantTypeAuthorizationCode = "authorization_code"
	grantTypeRefreshToken      = "refresh_token"
)

func newAuthClient(conf config.Trakt, transport http.RoundTripper) *authClient {
	return &authClient{
		baseURL: pathBaseAPI,
		client: &http.Client{
			Transport: transport,
		},
		conf: conf,
	}
}

func newAuthTransport(next http.RoundTripper, ac *authClient, clientID, tokenFilePath string, logger *slog.Logger) *authTransport {
	return &authTransport{
		authClient:    ac,
		next:          next,
		clientID:      clientID,
		tokenFilePath: tokenFilePath,
		logger:        logger,
	}
}

func (at *accessToken) isExpired() bool {
	return at != nil && time.Now().After(at.expiresAt)
}

func (ac *authClient) getAuthCodes(ctx context.Context) (*authCodesResponse, error) {
	b, err := json.Marshal(authCodesBody{
		ClientID: *ac.conf.ClientID,
	})
	if err != nil {
		return nil, fmt.Errorf("failure marshaling auth codes body: %w", err)
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, ac.client, http.MethodPost, ac.baseURL, pathAuthCodes, nil, body, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*authCodesResponse](resp.Body)
}

func (ac *authClient) getAccessToken(ctx context.Context, grantType, secret string) (*accessToken, error) {
	atb := authTokensBody{
		ClientID:     *ac.conf.ClientID,
		ClientSecret: *ac.conf.ClientSecret,
		GrantType:    grantType,
	}
	if grantType == grantTypeAuthorizationCode {
		atb.Code = secret
	} else {
		atb.RefreshToken = secret
	}
	b, err := json.Marshal(atb)
	if err != nil {
		return nil, fmt.Errorf("failure marshaling auth tokens body: %w", err)
	}
	body := bytes.NewReader(b)
	resp, err := doRequest(ctx, ac.client, http.MethodPost, ac.baseURL, pathAuthTokens, nil, body, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	authTokens, err := decodeJSON[*authTokensResponse](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding auth tokens response: %w", err)
	}
	return authTokens.toAccessToken(), nil
}

// pollAccessToken implements the polling side of Trakt's OAuth device
// authorization grant (RFC 8628): it repeatedly submits the device code
// until the user approves it (200), rejects it, or the code expires.
// A 400 response means the user has not approved the code yet. 429 (poll
// too fast) is already retried with backoff by the underlying
// retryTransport, so it never reaches this loop.
func (ac *authClient) pollAccessToken(ctx context.Context, deviceCode string, interval, expiresIn int) (*accessToken, error) {
	b, err := json.Marshal(authTokensBody{
		Code:         deviceCode,
		ClientID:     *ac.conf.ClientID,
		ClientSecret: *ac.conf.ClientSecret,
		GrantType:    grantTypeAuthorizationCode,
	})
	if err != nil {
		return nil, fmt.Errorf("failure marshaling auth tokens body: %w", err)
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device authorization code expired before it was approved")
		}
		body := bytes.NewReader(b)
		resp, err := doRequest(ctx, ac.client, http.MethodPost, ac.baseURL, pathAuthTokens, nil, body, nil, http.StatusOK, http.StatusBadRequest)
		if err != nil {
			var statusErr *UnexpectedStatusCodeError
			if errors.As(err, &statusErr) {
				switch statusErr.Got {
				case http.StatusNotFound:
					return nil, fmt.Errorf("device authorization code not found: %w", err)
				case http.StatusConflict:
					return nil, fmt.Errorf("device authorization code was already approved: %w", err)
				case http.StatusGone:
					return nil, fmt.Errorf("device authorization code expired: %w", err)
				case http.StatusTeapot:
					return nil, fmt.Errorf("device authorization code was denied: %w", err)
				}
			}
			return nil, err
		}
		if resp.StatusCode == http.StatusBadRequest {
			resp.Body.Close()
			continue
		}
		authTokens, err := decodeJSON[*authTokensResponse](resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failure decoding auth tokens response: %w", err)
		}
		return authTokens.toAccessToken(), nil
	}
}

func (atr *authTokensResponse) toAccessToken() *accessToken {
	return &accessToken{
		token:        atr.AccessToken,
		refreshToken: atr.RefreshToken,
		expiresAt:    time.Unix(int64(atr.CreatedAt+atr.ExpiresIn), 0),
	}
}

func (at *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if at.accessToken == nil {
		stored, err := loadToken(at.tokenFilePath)
		if err != nil {
			return nil, fmt.Errorf("failure loading persisted trakt token: %w", err)
		}
		if stored != nil {
			at.accessToken = stored.toAccessToken()
		}
	}
	if at.accessToken == nil {
		accessToken, err := at.bootstrap(ctx)
		if err != nil {
			return nil, fmt.Errorf("failure performing trakt device authorization: %w", err)
		}
		at.accessToken = accessToken
		if err = at.persist(); err != nil {
			return nil, err
		}
	}
	if at.accessToken.isExpired() {
		accessToken, err := at.authClient.getAccessToken(ctx, grantTypeRefreshToken, at.accessToken.refreshToken)
		if err != nil {
			return nil, fmt.Errorf("failure exchanging trakt refresh token for access token: %w", err)
		}
		at.accessToken = accessToken
		if err = at.persist(); err != nil {
			return nil, err
		}
	}
	req.Header.Set("authorization", "Bearer "+at.accessToken.token)
	req.Header.Set("trakt-api-key", at.clientID)
	req.Header.Set("trakt-api-version", "2")
	return at.next.RoundTrip(req)
}

// bootstrap runs Trakt's OAuth device authorization grant end to end. It
// requires a one-off manual approval by a human (opening verificationURL and
// entering userCode), since Trakt no longer supports scripted email/password
// sign-in on their website. The resulting token is persisted by the caller
// so this only needs to happen once per token file, until the refresh token
// is revoked or lost.
func (at *authTransport) bootstrap(ctx context.Context) (*accessToken, error) {
	authCodes, err := at.authClient.getAuthCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failure generating auth codes: %w", err)
	}
	at.logger.Warn(
		"manual trakt authorization required: open the verification url in a browser, sign in, and enter the code to continue",
		"url", authCodes.VerificationURL,
		"code", authCodes.UserCode,
		"expiresInSeconds", authCodes.ExpiresIn,
	)
	accessToken, err := at.authClient.pollAccessToken(ctx, authCodes.DeviceCode, authCodes.Interval, authCodes.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("failure polling for trakt access token: %w", err)
	}
	at.logger.Info("trakt device authorization approved")
	return accessToken, nil
}

func (at *authTransport) persist() error {
	return saveToken(at.tokenFilePath, &storedToken{
		AccessToken:  at.accessToken.token,
		RefreshToken: at.accessToken.refreshToken,
		ExpiresAt:    at.accessToken.expiresAt,
	})
}
