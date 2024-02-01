package client

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_scrapeSelectionAttribute(t *testing.T) {
	type args struct {
		body       io.ReadCloser
		clientName string
		selector   string
		attribute  string
	}
	dummyBody := io.NopCloser(strings.NewReader(`<html><body><div class="test" data-test="test"></div></body></html>`))
	tests := []struct {
		name       string
		args       args
		assertions func(*assert.Assertions, *string, error)
	}{
		{
			name: "success",
			args: args{
				body:       dummyBody,
				clientName: "test",
				selector:   ".test",
				attribute:  "data-test",
			},
			assertions: func(assertions *assert.Assertions, result *string, err error) {
				assertions.Nil(err)
				assertions.Equal("test", *result)
			},
		},
		{
			name: "failure creating goquery document",
			args: args{
				body: &stuckReadCloser{},
			},
			assertions: func(assertions *assert.Assertions, result *string, err error) {
				assertions.Nil(result)
				assertions.NotNil(err)
				assertions.ErrorContains(err, "failure creating goquery document")
			},
		},
		{
			name: "failure scraping",
			args: args{
				body:       dummyBody,
				clientName: "test",
				selector:   ".invalid",
				attribute:  "data-test",
			},
			assertions: func(assertions *assert.Assertions, result *string, err error) {
				assertions.Nil(result)
				assertions.NotNil(err)
				assertions.ErrorContains(err, "failure scraping")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scrapeSelectionAttribute(tt.args.body, tt.args.clientName, tt.args.selector, tt.args.attribute)
			tt.assertions(assert.New(t), result, err)
		})
	}
}

type stuckReadCloser struct{}

func (*stuckReadCloser) Read([]byte) (int, error) {
	return 0, nil
}

func (*stuckReadCloser) Close() error {
	return nil
}
