package client

import (
	"io"
	"log"
)

func DrainBody(body io.ReadCloser) {
	err := body.Close()
	if err != nil {
		log.Fatalf("error closing response body: %v", err)
	}
}
