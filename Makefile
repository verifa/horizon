
CI_CMD := go run ./cmd/ci/ci.go

.PHONY: generate
generate:
	$(CI_CMD) -generate

.PHONY: pr
pr:
	$(CI_CMD) -pr

.PHONY: lint
lint:
	$(CI_CMD) -lint

.PHONY: test
test:
	$(CI_CMD) -test

.PHONY: ci
ci: pr
