BINARY := print-service

LDFLAGS := -s -w
GOFILES := $(shell find . -name '*.go')

.PHONY: all build run test vet fmt tidy build-all docker clean

all: fmt vet test build

# build the binary for the host platform
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

run:
	CGO_ENABLED=0 go run .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w $(GOFILES)

tidy:
	go mod tidy

# fully static release binaries for the target devices
build-all:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm   GOARM=6 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-armv6 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64         go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64         go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 .

docker:
	docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v6 \
		-t ghcr.io/kayorddx/print-service:local .

clean:
	rm -rf bin dist
