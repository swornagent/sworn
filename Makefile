VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet fmt clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/sworn ./cmd/sworn

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin dist
