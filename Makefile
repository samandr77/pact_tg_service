.PHONY: gen build lint test check

gen:
	protoc --proto_path=proto \
	       --go_out=gen --go_opt=paths=source_relative \
	       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
	       telegram.proto

build:
	go build ./cmd/server/...

lint:
	golangci-lint run ./...

test:
	go test -race ./...

check: lint test build
