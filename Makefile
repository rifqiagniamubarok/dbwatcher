BINARY := bin/dbwatch
IMAGE  := dbwatch:dev

.PHONY: build test run clean docker-build

build:
	go build -o $(BINARY) ./cmd/dbwatch

test:
	go test -race ./...

run:
	go run ./cmd/dbwatch

clean:
	rm -rf bin/

docker-build:
	docker build -t $(IMAGE) .
