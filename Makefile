.PHONY: dev down test vet build fmt web-install web-dev web-build

dev:
	docker compose up --build

down:
	docker compose down

test:
	go test -race ./...

vet:
	go vet ./...

build:
	go build ./cmd/...

fmt:
	gofmt -w cmd internal

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

.PHONY: verify
verify: test vet build web-build
