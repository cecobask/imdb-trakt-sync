package trakt

import (
	"fmt"
)

type UnexpectedStatusCodeError struct {
	Got  int
	Want []int
}

func (e *UnexpectedStatusCodeError) Error() string {
	return fmt.Sprintf("unexpected status code: got %d, want one of %d", e.Got, e.Want)
}

func NewUnexpectedStatusCodeError(got int, want ...int) error {
	return &UnexpectedStatusCodeError{
		Got:  got,
		Want: want,
	}
}

type AccountLimitExceededError struct{}

func (e *AccountLimitExceededError) Error() string {
	return "trakt account limit exceeded, more info here: https://forums.trakt.tv/t/freemium-experience-more-features-for-all-with-usage-limits/41641"
}

func NewAccountLimitExceededError() error {
	return &AccountLimitExceededError{}
}

type ListNotFoundError struct {
	Slug string
}

func (e *ListNotFoundError) Error() string {
	return fmt.Sprintf("list with slug %s could not be found", e.Slug)
}

func NewListNotFoundError(slug string) error {
	return &ListNotFoundError{
		Slug: slug,
	}
}
