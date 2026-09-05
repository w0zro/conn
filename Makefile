# What conn's t runs here: the tests the way they are run before a commit,
# vetted and under the race detector. conn believes a Makefile's test
# target over its guess for a go.mod, which would be go test ./... alone.
.PHONY: test
test:
	go vet ./...
	go test -race ./...
