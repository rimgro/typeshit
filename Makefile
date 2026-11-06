BIN := typeshit

.PHONY: build run test race fmt vet clean

build:
	go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

run:
	go run ./cmd/server -addr 127.0.0.1:8080 -data ./data

test:
	go test ./...

race:
	go test -race ./...

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...

clean:
	rm -f $(BIN)
