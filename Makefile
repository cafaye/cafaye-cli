BINARY=cafaye

.PHONY: build test fmt tidy run

build:
	go build -o bin/$(BINARY) .

test:
	go test ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

run:
	go run . --help
