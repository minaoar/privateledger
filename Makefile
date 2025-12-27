.PHONY: build build-all run clean test

BINARY=privateledger
VERSION?=0.1.0

build:
	go build -o $(BINARY) ./cmd/server

build-all:
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/server
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 ./cmd/server
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 ./cmd/server
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe ./cmd/server

run: build
	./$(BINARY)

test:
	go test -v ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/
	rm -f config.json
	rm -f privateledger.db
