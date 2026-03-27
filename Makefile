BINARY=cafaye

.PHONY: build test test-cmd test-repeat test-repeat-cmd fmt tidy run verify ci-local ci-local-all ci-local-retry

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

ci-local:
	npx -y @redwoodjs/agent-ci run --pause-on-failure --workflow .github/workflows/ci.yml

ci-local-all:
	npx -y @redwoodjs/agent-ci run --all --pause-on-failure --no-matrix

ci-local-retry:
	@if [ -z "$(RUNNER)" ]; then \
		echo "usage: make ci-local-retry RUNNER=<runner-name>"; \
		exit 1; \
	fi
	npx -y @redwoodjs/agent-ci retry --name "$(RUNNER)"
