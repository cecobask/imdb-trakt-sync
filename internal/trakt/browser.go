package trakt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/cecobask/imdb-trakt-sync/internal/config"
)

const (
	pathActivate          = "/activate"
	pathActivateAuthorize = "/activate/authorize"
	pathAuthSignIn        = "/auth/signin"
)

type Browser struct {
	baseURL string
	client  *http.Client
	conf    config.Trakt
}

type BrowserClient interface {
	BrowseSignIn(ctx context.Context) (*string, error)
	SignIn(ctx context.Context, authenticityToken string) error
	BrowseActivate(ctx context.Context) (*string, error)
	Activate(ctx context.Context, userCode, authenticityToken string) (*string, error)
	ActivateAuthorize(ctx context.Context, authenticityToken string) error
}

func NewBrowser(conf config.Trakt, transport http.RoundTripper) BrowserClient {
	jar, _ := cookiejar.New(nil)
	return &Browser{
		baseURL: "https://trakt.tv",
		client: &http.Client{
			Jar:       jar,
			Transport: transport,
		},
		conf: conf,
	}
}

func (c *Browser) BrowseSignIn(ctx context.Context) (*string, error) {
	resp, err := doRequest(ctx, c.client, http.MethodGet, c.baseURL, pathAuthSignIn, nil, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return selectorAttributeScrape(resp.Body, "#new_user > input[name=authenticity_token]", "value")
}

func (c *Browser) SignIn(ctx context.Context, authenticityToken string) error {
	data := url.Values{}
	data.Set("authenticity_token", authenticityToken)
	data.Set("user[login]", *c.conf.Email)
	data.Set("user[password]", *c.conf.Password)
	data.Set("user[remember_me]", "1")
	body := strings.NewReader(data.Encode())
	headers := map[string]string{
		"content-type": "application/x-www-form-urlencoded",
	}
	resp, err := doRequest(ctx, c.client, http.MethodPost, c.baseURL, pathAuthSignIn, nil, body, headers, http.StatusOK)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Browser) BrowseActivate(ctx context.Context) (*string, error) {
	response, err := doRequest(ctx, c.client, http.MethodGet, c.baseURL, pathActivate, nil, http.NoBody, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return selectorAttributeScrape(response.Body, "#auth-form-wrapper > form.form-signin > input[name=authenticity_token]", "value")
}

func (c *Browser) Activate(ctx context.Context, userCode, authenticityToken string) (*string, error) {
	data := url.Values{}
	data.Set("authenticity_token", authenticityToken)
	data.Set("code", userCode)
	data.Set("commit", "Continue")
	body := strings.NewReader(data.Encode())
	headers := map[string]string{
		"content-type": "application/x-www-form-urlencoded",
	}
	resp, err := doRequest(ctx, c.client, http.MethodPost, c.baseURL, pathActivate, nil, body, headers, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return selectorAttributeScrape(resp.Body, "#auth-form-wrapper > div.form-signin.less-top > div > form:nth-child(1) > input[name=authenticity_token]:nth-child(1)", "value")
}

func (c *Browser) ActivateAuthorize(ctx context.Context, authenticityToken string) error {
	data := url.Values{}
	data.Set("authenticity_token", authenticityToken)
	data.Set("commit", "Yes")
	body := strings.NewReader(data.Encode())
	headers := map[string]string{
		"content-type": "application/x-www-form-urlencoded",
	}
	resp, err := doRequest(ctx, c.client, http.MethodPost, c.baseURL, pathActivateAuthorize, nil, body, headers, http.StatusOK)
	if err != nil {
		return err
	}
	return selectorExists(resp.Body, "a[href='/logout']")
}

func doRequest(ctx context.Context, client *http.Client, method, baseURL, path string, query map[string][]string, body io.Reader, headers map[string]string, statusCodes ...int) (*http.Response, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failure parsing base url: %w", err)
	}
	u = u.JoinPath(path)
	q := u.Query()
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failure creating http request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed sending http request: %w", err)
	}
	if !slices.Contains(statusCodes, resp.StatusCode) {
		return nil, NewUnexpectedStatusCodeError(resp.StatusCode, statusCodes...)
	}
	return resp, nil
}

func selectorExists(body io.ReadCloser, selector string) error {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return fmt.Errorf("failure creating goquery document from response: %w", err)
	}
	if doc.Find(selector).Length() == 0 {
		return fmt.Errorf("failure finding selector %s", selector)
	}
	return nil
}

func selectorAttributeScrape(body io.ReadCloser, selector, attribute string) (*string, error) {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from response: %w", err)
	}
	value, ok := doc.Find(selector).Attr(attribute)
	if !ok {
		return nil, fmt.Errorf("failure scraping response for selector %s and attribute %s", selector, attribute)
	}
	return &value, nil
}
