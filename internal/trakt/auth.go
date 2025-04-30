package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
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
	browserClient BrowserClient
	next          http.RoundTripper
	accessToken   *accessToken
	clientID      string
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

func newAuthTransport(next http.RoundTripper, ac *authClient, bc BrowserClient, clientID string) *authTransport {
	return &authTransport{
		authClient:    ac,
		browserClient: bc,
		next:          next,
		clientID:      clientID,
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
	resp, err := doRequest(ctx, ac.client, http.MethodPost, ac.baseURL, pathAuthCodes, body, nil, http.StatusOK)
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
	resp, err := doRequest(ctx, ac.client, http.MethodPost, ac.baseURL, pathAuthTokens, body, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	authTokens, err := decodeJSON[*authTokensResponse](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failure decoding auth tokens response: %w", err)
	}
	return authTokens.toAccessToken(), nil
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
		authCodes, err := at.authClient.getAuthCodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("failure generating auth codes: %w", err)
		}
		authenticityToken, err := at.browserClient.BrowseSignIn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failure simulating browse to the trakt sign in page: %w", err)
		}
		if err = at.browserClient.SignIn(ctx, *authenticityToken); err != nil {
			return nil, fmt.Errorf("failure simulating trakt sign in form submission: %w", err)
		}
		authenticityToken, err = at.browserClient.BrowseActivate(ctx)
		if err != nil {
			return nil, fmt.Errorf("failure simulating browse to the trakt device activation page: %w", err)
		}
		authenticityToken, err = at.browserClient.Activate(ctx, authCodes.UserCode, *authenticityToken)
		if err != nil {
			return nil, fmt.Errorf("failure simulating trakt device activation form submission: %w", err)
		}
		if err = at.browserClient.ActivateAuthorize(ctx, *authenticityToken); err != nil {
			return nil, fmt.Errorf("failure simulating trakt api app allowlisting: %w", err)
		}
		accessToken, err := at.authClient.getAccessToken(ctx, grantTypeAuthorizationCode, authCodes.DeviceCode)
		if err != nil {
			return nil, fmt.Errorf("failure exchanging trakt device code for access token: %w", err)
		}
		at.accessToken = accessToken
	}
	if at.accessToken.isExpired() {
		accessToken, err := at.authClient.getAccessToken(ctx, grantTypeRefreshToken, at.accessToken.refreshToken)
		if err != nil {
			return nil, fmt.Errorf("failure exchanging trakt refresh token for access token: %w", err)
		}
		at.accessToken = accessToken
	}
	req.Header.Set("authorization", "Bearer "+at.accessToken.token)
	req.Header.Set("trakt-api-key", at.clientID)
	req.Header.Set("trakt-api-version", "2")
	return at.next.RoundTrip(req)
}
