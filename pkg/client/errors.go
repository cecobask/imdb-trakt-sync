package client

import (
	"fmt"
)

type ApiError struct {
	httpMethod string
	url        string
	StatusCode int
	details    string
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("http request %s %s returned status code %d: %s", e.httpMethod, e.url, e.StatusCode, e.details)
}
