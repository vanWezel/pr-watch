VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := prwatch

.PHONY: build install test vet fmt clean run

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

test:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

run: build
	./$(BIN)

clean:
	rm -f $(BIN)
