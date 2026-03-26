BINARY=cafaye

.PHONY: build test test-cmd test-repeat test-repeat-cmd fmt tidy run verify

build:
	go build -o bin/$(BINARY) .

test:
	go test ./...

test-cmd:
	go test ./cmd/...

test-repeat:
	@for i in 1 2 3; do \
		echo "==> pass $$i/3 (all tests)"; \
		go test ./... || exit $$?; \
	done

test-repeat-cmd:
	@for i in 1 2 3; do \
		echo "==> pass $$i/3 (cmd tests)"; \
		go test ./cmd/... || exit $$?; \
	done

fmt:
	gofmt -w .

tidy:
	go mod tidy

run:
	go run . --help

verify: fmt test
