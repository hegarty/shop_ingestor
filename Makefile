.PHONY: build test vet lint fmt fmt-check tidy migrate-up migrate-down docker ci

build:
	go build ./...

test:
	go test ./... -race -count=1

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "The following files are not gofmt'd:"; \
		gofmt -l .; \
		exit 1; \
	fi

tidy:
	go mod tidy

migrate-up:
	go run ./cmd/migrate --direction up

migrate-down:
	go run ./cmd/migrate --direction down --steps 1

docker:
	docker build -t shop-ingestor:local .

ci: fmt-check vet build test
