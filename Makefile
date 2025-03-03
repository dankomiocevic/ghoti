#-----------------------------------------------------------------------------------------------------------------------
# Variables (https://www.gnu.org/software/make/manual/html_node/Using-Variables.html#Using-Variables)
#-----------------------------------------------------------------------------------------------------------------------
.DEFAULT_GOAL := help

BINARY_NAME = ghoti
BUILD_DIR ?= $(CURDIR)/dist
GO_BIN ?= $(shell go env GOPATH)/bin
GO_PACKAGES := $(shell go list ./... | grep -vE "vendor")

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
.PHONY: test generate-mocks

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

test-bench: generate-mocks ## Run benchmark tests. See https://pkg.go.dev/cmd/go#hdr-Testing_flags
	${call print, "Running benchmark tests"}
	@go test ./... -bench . -benchtime 5s -timeout 0 -run=XXX -cpu 1 -benchmem

#-----------------------------------------------------------------------------------------------------------------------
# Helpers
#-----------------------------------------------------------------------------------------------------------------------
define print
	@printf "${TEXT_ENABLE_STANDOUT}${COLOR_WHITE} 🚀 ${COLOR_BLUE} %-70s ${COLOR_WHITE} ${TEXT_DISABLE_STANDOUT}\n" $(1)
endef
