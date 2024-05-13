.PHONY: *

build:
	@go build -o build/its main.go

configure:
	@./build/its configure

sync:
	@./build/its sync

html-coverage:
	@go tool cover -html=coverage.out

lint:
	@golangci-lint run

lint-fix:
	@golangci-lint run --fix

test:
	@go test -race -coverprofile=coverage.out -shuffle on ./...
