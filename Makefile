#-----------------------------------------------------------------------------------------------------------------------
# Variables (https://www.gnu.org/software/make/manual/html_node/Using-Variables.html#Using-Variables)
#-----------------------------------------------------------------------------------------------------------------------
.DEFAULT_GOAL := help

BINARY_NAME = ghoti
BUILD_DIR ?= $(CURDIR)/dist
GO_BIN ?= $(shell go env GOPATH)/bin
GO_PACKAGES := $(shell go list ./... | grep -vE "vendor")

# Defaults for the `test-race` target
RACE_COUNT ?= 3
RACE_PACKAGES ?= $(GO_PACKAGES)

GORELEASER_VERSION ?= v2.18.2
RELEASE_SKIP ?= sign,publish

# Colors for the printf
RESET = $(shell tput sgr0)
COLOR_WHITE = $(shell tput setaf 7)
COLOR_BLUE = $(shell tput setaf 4)
TEXT_ENABLE_STANDOUT = $(shell tput smso)
TEXT_DISABLE_STANDOUT = $(shell tput rmso)

#-----------------------------------------------------------------------------------------------------------------------
# Rules (https://www.gnu.org/software/make/manual/html_node/Rule-Introduction.html#Rule-Introduction)
#-----------------------------------------------------------------------------------------------------------------------
.PHONY: help clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

clean: ## Clean project files
	${call print, "Removing ${BUILD_DIR}/${BINARY_NAME}"}
	@rm "${BUILD_DIR}/${BINARY_NAME}"
	@go clean -x -r -i

#-----------------------------------------------------------------------------------------------------------------------
# Building & Installing
#-----------------------------------------------------------------------------------------------------------------------
.PHONY: build install

build: ## Build Ghoti binary. Build directory can be overridden using BUILD_DIR="desired/path", default is ".dist/". Usage `BUILD_DIR="." make build`
	${call print, "Building Ghoti binary within ${BUILD_DIR}/${BINARY_NAME}"}
	@go build -v -o "${BUILD_DIR}/${BINARY_NAME}" "$(CURDIR)/cmd/ghoti"

install: ## Install Ghoti within $GO_BIN. Ensure that $GO_BIN is available on the $PATH to run the executable from anywhere
	${call print, "Installing Ghoti binary within ${GO_BIN}"}
	@go install -v "$(CURDIR)/cmd/${BINARY_NAME}"

#-----------------------------------------------------------------------------------------------------------------------
# Checks
#-----------------------------------------------------------------------------------------------------------------------
.PHONY: lint

lint: $(GO_BIN)/golangci-lint ## Lint Go source files
	${call print, "Linting Go source files"}
	@golangci-lint run -v --fix -c .golangci.yaml ./...

#-----------------------------------------------------------------------------------------------------------------------
# Tests
#-----------------------------------------------------------------------------------------------------------------------
.PHONY: test test-race generate-mocks

test: generate-mocks ## Run all tests. To run a specific test, pass the FILTER var. Usage `make test FILTER="TestCheckLogs"`
	# To skip integration tests, define SHORT. Usage `make test SHORT=1`
	${call print, "Running tests"}
ifdef SHORT
	@go test -race \
		  -short \
			-run "$(FILTER)" \
			-coverpkg=./... \
			-coverprofile=coverageunit.tmp.out \
			-covermode=atomic \
			-count=1 \
			-timeout=10m \
			${GO_PACKAGES}
else
	@go test -race \
			-run "$(FILTER)" \
			-coverpkg=./... \
			-coverprofile=coverageunit.tmp.out \
			-covermode=atomic \
			-count=1 \
			-timeout=10m \
			${GO_PACKAGES}
endif
	@cat coverageunit.tmp.out | grep -v "mock" > coverageunit.out
	@rm coverageunit.tmp.out

test-race: generate-mocks ## Run tests repeatedly under the race detector. Override RACE_COUNT, RACE_PACKAGES or FILTER. Usage `make test-race RACE_PACKAGES=./internal/slots/`
	# Repeated runs give the detector more interleavings to observe than the
	# single pass in `make test`. It only reports races on executed paths, so
	# a clean run is evidence about coverage, not a proof of safety.
	${call print, "Running tests under the race detector ($(RACE_COUNT) runs)"}
	@go test -race \
			-run "$(FILTER)" \
			-count=$(RACE_COUNT) \
			-timeout=10m \
			$(RACE_PACKAGES)

test-bench: generate-mocks ## Run benchmark tests. See https://pkg.go.dev/cmd/go#hdr-Testing_flags
	${call print, "Running benchmark tests"}
	@go test ./... -bench . -benchtime 5s -timeout 0 -run=XXX -cpu 1 -benchmem

#-----------------------------------------------------------------------------------------------------------------------
# Release
#-----------------------------------------------------------------------------------------------------------------------
# Releases are built by the Release workflow when a v* tag is pushed, see
# .goreleaser.yaml. These targets are for checking the configuration and the
# artefacts locally, nothing is published from here.
.PHONY: release-check release-snapshot release-notes

$(GO_BIN)/goreleaser:
	@go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

release-check: $(GO_BIN)/goreleaser ## Validate .goreleaser.yaml
	${call print, "Checking GoReleaser configuration"}
	@$(GO_BIN)/goreleaser check

release-snapshot: $(GO_BIN)/goreleaser ## Build every release artefact into dist/ without publishing or signing. Usage `make release-snapshot RELEASE_SKIP=sign,publish,sbom`
	${call print, "Building release snapshot within ${BUILD_DIR}"}
	@$(GO_BIN)/goreleaser release --snapshot --clean --skip=$(RELEASE_SKIP)

release-notes: ## Print the CHANGELOG.md section that becomes the release body. Usage `make release-notes VERSION=v0.2.0`
	@./scripts/release-notes.sh "$(VERSION)"

#-----------------------------------------------------------------------------------------------------------------------
# Helpers
#-----------------------------------------------------------------------------------------------------------------------
define print
	@printf "${TEXT_ENABLE_STANDOUT}${COLOR_WHITE} 🚀 ${COLOR_BLUE} %-70s ${COLOR_WHITE} ${TEXT_DISABLE_STANDOUT}\n" $(1)
endef
