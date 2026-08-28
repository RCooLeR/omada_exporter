.PHONY: build docker-build generate-metrics-table lint release-check test test-race verify \
	bridge-build bridge-generate-metrics-table bridge-lint bridge-test bridge-test-race bridge-verify

build: bridge-build

lint: bridge-lint

test: bridge-test

test-race: bridge-test-race

verify: bridge-verify release-check

docker-build:
	$(MAKE) -C bridge docker-build

release-check:
	goreleaser check

generate-metrics-table:
	cd bridge && go run . --host dummy --client-id dummy --secret-id dummy --username dummy --password dummy mdocs > ../gen-metrics-table.md

bridge-lint:
	$(MAKE) -C bridge lint

bridge-build:
	$(MAKE) -C bridge build

bridge-test:
	$(MAKE) -C bridge test

bridge-test-race:
	$(MAKE) -C bridge test-race

bridge-verify:
	$(MAKE) -C bridge verify

bridge-generate-metrics-table:
	$(MAKE) -C bridge generate-metrics-table
