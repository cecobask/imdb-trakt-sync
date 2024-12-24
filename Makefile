.PHONY: *

build:
	@go build -o build/its main.go

package:
	@docker buildx build -t its:dev --platform=linux/amd64 .

configure:
	@./build/its configure

sync:
	@./build/its sync

sync-container:
	@docker run -it --rm --platform=linux/amd64 --env-file=.env its:dev

html-coverage:
	@go tool cover -html=coverage.out

lint:
	@golangci-lint run

lint-fix:
	@golangci-lint run --fix

test:
	@go test -race -coverprofile=coverage.out -shuffle on ./...
