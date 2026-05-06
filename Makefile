.PHONY: lint generate-metrics-table bridge-lint bridge-generate-metrics-table

lint: bridge-lint

generate-metrics-table:
	cd bridge && go run . --host dummy --client-id dummy --secret-id dummy --username dummy --password dummy mdocs > ../gen-metrics-table.md

bridge-lint:
	$(MAKE) -C bridge lint

bridge-generate-metrics-table:
	$(MAKE) -C bridge generate-metrics-table
