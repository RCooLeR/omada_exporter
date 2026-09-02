.PHONY: build docker-build generate-dashboards generate-metrics-table lint test test-race vet verify

build:
	go build -trimpath ./...

docker-build:
	docker buildx build --load --tag omada-exporter:dev .

lint:
	golangci-lint run

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

verify: lint test vet build

generate-metrics-table:
	go run . --host https://dummy.invalid --client-id dummy --secret-id dummy --username dummy --password dummy mdocs > gen-metrics-table.md

generate-dashboards:
	go generate ./docs/dashboards
