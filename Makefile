.PHONY: verify core web images
verify: core web
core:
	@test -n "$$VMCP_TEST_DATABASE_URL" || (echo 'Set VMCP_TEST_DATABASE_URL to a disposable PostgreSQL instance' >&2; exit 1)
	@test -n "$$VMCP_TEST_REDIS_URL" || (echo 'Set VMCP_TEST_REDIS_URL to a disposable Redis instance' >&2; exit 1)
	cd core && go vet ./...
	cd core && VMCP_REQUIRE_INTEGRATION=1 VMCP_REQUIRE_SDK_INTEROP=1 go test -race -count=1 -json ./... > ../verification.jsonl
	python3 scripts/check-test-results.py verification.jsonl
web:
	cd web && npm test && npm run lint && npm run build && npm run validate:api && npm run test:browser
images:
	docker build --build-arg SERVICE=control-plane -t vmcp/control-plane:local core
	docker build --build-arg SERVICE=data-plane -t vmcp/data-plane:local core
	docker build --build-arg SERVICE=migrate -t vmcp/migrate:local core
	docker build --build-arg SERVICE=audit-exporter -t vmcp/audit-exporter:local core
