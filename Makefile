# atl — spec refresh, catalog build, binary build.
# Specs are Atlassian's published OpenAPI; the embedded catalog is a derived artifact.

SPECS_DIR := specs

.PHONY: specs catalog build test

## specs: re-pull all Atlassian OpenAPI specs (see specs/SOURCES.md)
specs:
	curl -sL -o $(SPECS_DIR)/jira-cloud.v3.json       "https://developer.atlassian.com/cloud/jira/platform/swagger-v3.v3.json"
	curl -sL -o $(SPECS_DIR)/jira-software.v3.json     "https://developer.atlassian.com/cloud/jira/software/swagger.v3.json"
	curl -sL -o $(SPECS_DIR)/confluence-cloud.v1.json  "https://developer.atlassian.com/cloud/confluence/swagger.v3.json"
	curl -sL -o $(SPECS_DIR)/confluence-cloud.v2.json  "https://developer.atlassian.com/cloud/confluence/openapi-v2.v3.json"
	curl -sL -o $(SPECS_DIR)/bitbucket-cloud.json      "https://api.bitbucket.org/swagger.json"
	@echo "specs refreshed"

## catalog: distill the embedded operation catalog from specs/*.json
catalog:
	go run ./scripts/build-catalog

## build: build the atl binary
build:
	go build -o atl .

## test: run the unit tests
test:
	go test ./...
