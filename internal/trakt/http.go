package trakt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
)

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
