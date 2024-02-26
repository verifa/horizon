
CI_CMD := go run ./cmd/ci/*.go

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
