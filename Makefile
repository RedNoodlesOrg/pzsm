BINARY := pzsm
PKG    := ./cmd/pzsm
CONFIG := config.yaml

.DEFAULT_GOAL := build
.PHONY: build dev run test vet tidy clean generate

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) $(PKG)

dev:
	go build -tags devbypass -o $(BINARY) $(PKG)

run:
	go run -tags devbypass $(PKG) --config $(CONFIG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)

generate:
	go generate ./...
