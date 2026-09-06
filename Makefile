.PHONY: all build ui binary clean test vet check

all: build

build: ui binary

binary:
	go build -o pogu ./cmd/pogu

ui:
	cd ui && npm ci && npm run build

test:
	go test -race ./...

vet:
	go vet ./...

check: vet test
	cd ui && npm ci && npm run check && npm test

clean:
	rm -f pogu
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
