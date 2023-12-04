.PHONY: *

build:
	go build -o build/syncer cmd/syncer/main.go

fmt:
	go fmt ./...

html-coverage:
	go tool cover -html=coverage.out

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

mocks:
	mockery

test:
	go test -coverpkg=./... -race -coverprofile=coverage.out -shuffle on ./...
