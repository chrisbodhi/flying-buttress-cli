.PHONY: build test test-race test-cover test-cover-html cover-summary lint vet fmt fmt-check tidy ci clean

# Default target: fast feedback loop.
test:
	go test ./...

# Race detector — slower but catches concurrency bugs.
test-race:
	go test -race ./...

# Coverage profile written to coverage.out.
test-cover:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	@$(MAKE) cover-summary

# Pretty per-package coverage breakdown.
cover-summary:
	@echo
	@echo "Coverage by package:"
	@go tool cover -func=coverage.out | tail -1
	@echo
	@go test -cover ./... 2>/dev/null | grep -E '^(ok|FAIL)' | awk '{printf "  %-40s %s\n", $$2, $$NF}'

# Open HTML coverage report in browser (or write coverage.html).
test-cover-html: test-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.html"

# Verify build, vet, and tests all pass — used in CI.
ci: vet test-race test-cover
	@echo "CI checks passed."

build:
	go build -o buttress .

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-clean:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

tidy:
	go mod tidy

clean:
	rm -f buttress fb coverage.out coverage.html
