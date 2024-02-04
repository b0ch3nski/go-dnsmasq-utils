.PHONY: test

test: ## Runs all unit tests
	go test -v -race -count='1' ./...
