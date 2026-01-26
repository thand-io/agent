.PHONY: help test test-short clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run all UI E2E tests
	go test -v -timeout 20m ./...

test-short: ## Run tests in short mode (skips UI tests)
	go test -short ./...

test-aws: ## Run only AWS elevation UI test
	go test -v -timeout 10m -run TestAWSElevationUI

test-approval: ## Run only approval workflow UI test
	go test -v -timeout 10m -run TestApprovalWorkflowUI

test-infra: ## Run infrastructure setup test
	go test -v -timeout 5m -run TestUIInfrastructureSetup

clean: ## Clean up generated files
	rm -f *.png *.jpg *.mp4
	find . -name 'thand-ui-test-*' -type d -exec rm -rf {} +

build-agent: ## Build the agent binary (required for tests)
	cd ../../.. && make build

setup: build-agent ## Complete setup: build agent

ci: ## Run tests in CI mode
	go test -v -timeout 20m -race ./...
