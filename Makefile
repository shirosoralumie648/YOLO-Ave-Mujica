.PHONY: test

test:
	GOCACHE=/tmp/go-build go test ./...
