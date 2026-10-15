VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all web server sim cli test run-sim dev clean

all: web server

web:
	cd web && npm ci && npx ng build

server:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/heliocontrol ./cmd/heliocontrol

sim:
	go build -o bin/heliosim ./cmd/heliosim

cli:
	go build -o bin/heliocli ./cmd/heliocli

test:
	go vet ./...
	go test ./...
	cd web && npx ng test --watch=false

# simulator + server, UI on http://localhost:8080
run-sim: server sim
	bin/heliosim -listen 127.0.0.1:4001 & \
	bin/heliocontrol -config dev.yaml

# Angular dev server with live reload (proxies /api to :8080)
dev:
	cd web && npx ng serve

clean:
	rm -rf bin web/dist/browser/*.js web/dist/browser/*.css
