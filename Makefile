VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X github.com/unmaykr-a/transit-tracker/internal/buildinfo.Version=$(VERSION) -X github.com/unmaykr-a/transit-tracker/internal/buildinfo.Commit=$(COMMIT)

.PHONY: all build web test test-go test-web lint run docker clean

all: web build

web:
	cd web && npm ci --no-audit --no-fund && npm run build
	find internal/api/dist -mindepth 1 ! -name .gitkeep -delete
	cp -R web/dist/. internal/api/dist/

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/bus ./cmd/bus

test: test-go test-web

test-go:
	go test -race ./...

test-web:
	cd web && npm run test

lint:
	test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal && exit 1)
	go vet ./...
	cd web && npm run lint && npm run typecheck

run:
	go run ./cmd/bus

docker:
	docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t bus:$(VERSION) .

clean:
	rm -rf bin web/dist
	find internal/api/dist -mindepth 1 ! -name .gitkeep -delete
