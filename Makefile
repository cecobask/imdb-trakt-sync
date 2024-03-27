.PHONY: *

build:
	go build -o build/its main.go

sync: build
	./build/its sync

fmt:
	go fmt ./...

html-coverage:
	go tool cover -html=coverage.out

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

test:
	go test -coverpkg=./... -race -coverprofile=coverage.out -shuffle on ./...
