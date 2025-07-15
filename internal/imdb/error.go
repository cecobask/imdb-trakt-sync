package imdb

import "fmt"

type UnexportableResourceError struct {
	URL string
}

func (e *UnexportableResourceError) Error() string {
	return fmt.Sprintf("resource at url %s is not exportable", e.URL)
}

func NewUnexportableResourceError(url string) error {
	return &UnexportableResourceError{
		URL: url,
	}
}
