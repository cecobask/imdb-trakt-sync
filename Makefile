.PHONY: *

build:
	go build -o build/its main.go

package:
	docker buildx build -t its:dev --platform=linux/amd64 .

configure:
	./build/its configure

sync:
	./build/its sync

sync-container:
	docker run -it --rm --platform=linux/amd64 --env-file=.env its:dev

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix
