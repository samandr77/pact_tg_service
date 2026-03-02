.PHONY: gen build lint test check

gen:
	protoc --go_out=gen --go-grpc_out=gen proto/telegram.proto

build:
	go build ./cmd/server/...

lint:
	golangci-lint run ./...

test:
	go test -race ./...

check: lint test build
