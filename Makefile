BIN := bin/osapi
IMG := ghcr.io/whitedukecmr/openstack-go-api

.PHONY: build test lint run docker
build:
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/osapi
test:
	go test -race -cover ./...
lint:
	go vet ./... && gofmt -l . | (! grep .)
run: build
	./$(BIN)
docker:
	docker build -t $(IMG):dev .
