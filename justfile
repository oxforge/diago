# Show all recipes
default:
    @just --list

# Build the CLI to bin/diago
build:
    go build -o bin/diago ./cmd/diago

# Install the CLI into $GOBIN (or $GOPATH/bin)
install:
    go install ./cmd/diago

# Run all Go unit tests
test:
    go test ./...

# Run golangci-lint
lint:
    golangci-lint run

# Regenerate schemas/{flow,sequence,class}.json from the Go spec structs
schemas:
    cd internal/schema && go generate

# Maintainer-only recipes, when present
import? 'dev.just'
