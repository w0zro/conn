# What conn's t, b and l run here, each the way it is done before a commit
# or a release. conn believes a Makefile's targets over its guesses for a
# go.mod — go test ./..., go build ./... and go vet ./... alone.
.PHONY: test build lint

# The tests, vetted and under the race detector.
test:
	go vet ./...
	go test -race ./...

# The binary the release builds, built the release's way — trimmed paths,
# stripped, stamped with the version git describes — to ./conn, which is
# ignored.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$$(git describe --tags --always --dirty)" -o conn .

# What the CI holds the tree to: formatted, and clean under the linters
# .golangci.yml pins — on Linux as well as here, since the process list is
# read by different files on each and a function only one of them calls is
# unused on the other.
lint:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "unformatted:"; echo "$$unformatted"; exit 1; fi
	golangci-lint run ./...
	GOOS=linux golangci-lint run ./...
