package client

import (
	"fmt"
)

type ApiError struct {
	clientName string
	httpMethod string
	url        string
	StatusCode int
	details    string
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("%s request %s %s returned status code %d: %s", e.clientName, e.httpMethod, e.url, e.StatusCode, e.details)
}
