BINARY    := bin/dbwatch
IMAGE     := dbwatch:dev
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

.PHONY: build test run clean docker-build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/dbwatch

test:
	go test -race ./...

run:
	go run ./cmd/dbwatch

clean:
	rm -rf bin/

docker-build:
	docker build -t $(IMAGE) .
