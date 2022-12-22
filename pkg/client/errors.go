package client

import (
	"fmt"
)

type ResourceNotFoundError struct {
	resourceType string
	resourceName string
	httpMethod   string
	url          string
}

func (e *ResourceNotFoundError) Error() string {
	return fmt.Sprintf("%s with name %s could not be found; originating request: %s %s", e.resourceType, e.resourceName, e.httpMethod, e.url)
}
