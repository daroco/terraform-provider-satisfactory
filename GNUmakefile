default: fmt lint test

build:
	go build -v ./...

install: build
	go install -v ./...

# gofmt the whole tree.
fmt:
	gofmt -s -w -e .

# Static analysis. golangci-lint config lives in .golangci.yml.
lint:
	go vet ./...
	golangci-lint run

# Unit tests (no game or terraform binary required).
test:
	go test -cover -timeout=120s ./...

# Acceptance tests against the in-memory mock. Needs the terraform CLI on PATH.
testacc:
	TF_ACC=1 go test -v -cover -timeout 20m ./internal/provider/

# Regenerate the Terraform Registry docs in docs/ from the provider schema and
# examples/. Needs the terraform CLI on PATH. `docs` is an alias for `generate`.
generate:
	cd tools && go generate ./...

docs: generate

# Run the in-memory mock world on :8090 for local development.
mock:
	go run ./cmd/mockserver

.PHONY: default build install fmt lint test testacc generate docs mock
