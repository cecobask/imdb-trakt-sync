package client

import (
	"fmt"
)

type ResourceNotFoundError struct {
	resourceType string
	resourceId   *string
}

func (e *ResourceNotFoundError) Error() string {
	if e.resourceId != nil {
		return fmt.Sprintf("%s with id %s could not be found", e.resourceType, *e.resourceId)
	}
	return fmt.Sprintf("%s could not be found", e.resourceType)
}

type ImdbError struct {
	httpMethod string
	url        string
	statusCode int
	details    string
}

func (e *ImdbError) Error() string {
	return fmt.Sprintf("imdb request %s %s returned unhealthy status code %d: %s", e.httpMethod, e.url, e.statusCode, e.details)
}

type TraktError struct {
	httpMethod string
	url        string
	statusCode int
	details    string
}

func (e *TraktError) Error() string {
	return fmt.Sprintf("trakt request %s %s returned unhealthy status code %d: %s", e.httpMethod, e.url, e.statusCode, e.details)
}
