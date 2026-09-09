# The tests, the build and the lint, each the way it is done before a
# commit or a release.
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
# .golangci.yml pins — on Linux as well as here, since the machine is read
# by a file for each platform, and a function only one of them calls is
# unused on the other.
lint:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "unformatted:"; echo "$$unformatted"; exit 1; fi
	golangci-lint run ./...
	GOOS=linux golangci-lint run ./...
