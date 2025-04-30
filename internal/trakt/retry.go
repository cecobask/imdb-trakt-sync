package trakt

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type retryTransport struct {
	next   http.RoundTripper
	logger *slog.Logger
}

func newRetryTransport(next http.RoundTripper, logger *slog.Logger) *retryTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &retryTransport{
		next:   next,
		logger: logger,
	}
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 1; attempt <= 5; attempt++ {
		resp, err := rt.next.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("failure sending http request: %w", err)
		}
		if resp.StatusCode == 420 {
			resp.Body.Close()
			return nil, NewAccountLimitExceededError()
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			retryAfterHeader, retryAfter := resp.Header.Get("retry-after"), 30
			if retryAfterHeader != "" {
				if retryAfter, err = strconv.Atoi(retryAfterHeader); err != nil {
					return nil, fmt.Errorf("failure parsing the value of header 'retry-after' to integer: %w", err)
				}
			}
			duration := time.Duration(retryAfter) * time.Second
			rt.logger.Error("rate limit reached, retrying request",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("attempt", attempt),
				slog.String("backoff", duration.String()),
			)
			time.Sleep(duration)
			continue
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			duration := time.Second
			rt.logger.Error("server error encountered, retrying request",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("attempt", attempt),
				slog.String("backoff", duration.String()),
			)
			time.Sleep(duration)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("reached max retry attempts for http request %s %s", req.Method, req.URL)
}

type rereader struct {
	io.Reader
	readBuf *bytes.Buffer
	backBuf *bytes.Buffer
}

func NewReader(r io.Reader) io.Reader {
	readBuf := bytes.Buffer{}
	readBuf.ReadFrom(r)
	backBuf := bytes.Buffer{}
	return rereader{
		Reader:  io.TeeReader(&readBuf, &backBuf),
		readBuf: &readBuf,
		backBuf: &backBuf,
	}
}

func (r rereader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		io.Copy(r.readBuf, r.backBuf)
	}
	return n, err
}
