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
.PHONY: build build-all install

build: ## Build Ghoti binary. Build directory can be overridden using BUILD_DIR="desired/path", default is ".dist/". Usage `BUILD_DIR="." make build`
	${call print, "Building Ghoti binary within ${BUILD_DIR}/${BINARY_NAME}"}
	@go build -v -o "${BUILD_DIR}/${BINARY_NAME}" "$(CURDIR)/cmd/ghoti"

build-all: ## Build Ghoti binary for all platforms. Usage `make build-all`
	${call print, "Building Ghoti binary for all platforms"}
	@echo "Building for Linux..."
	@GOOS=linux GOARCH=amd64 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-linux-amd64" "$(CURDIR)/cmd/ghoti"
	@GOOS=linux GOARCH=arm64 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-linux-arm64" "$(CURDIR)/cmd/ghoti"
	@GOOS=linux GOARCH=386 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-linux-386" "$(CURDIR)/cmd/ghoti"
	@echo "Building for Windows..."
	@GOOS=windows GOARCH=amd64 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-windows-amd64.exe" "$(CURDIR)/cmd/ghoti"
	@GOOS=windows GOARCH=386 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-windows-386.exe" "$(CURDIR)/cmd/ghoti"
	@echo "Building for Mac..."
	@GOOS=darwin GOARCH=amd64 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-darwin-amd64" "$(CURDIR)/cmd/ghoti"
	@GOOS=darwin GOARCH=arm64 go build -v -o "${BUILD_DIR}/${BINARY_NAME}-darwin-arm64" "$(CURDIR)/cmd/ghoti"
	@echo "Build done"

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
# Release
#-----------------------------------------------------------------------------------------------------------------------

release: build-all
	@echo "Creating release packages..."
	@# Linux packages
	@for arch in amd64 arm64 386; do \
		echo "Creating package for linux-$$arch..."; \
		package_dir="${BUILD_DIR}/tmp/ghoti-linux-$$arch"; \
		mkdir -p "$$package_dir"; \
		cp "${BUILD_DIR}/${BINARY_NAME}-linux-$$arch" "$$package_dir/${BINARY_NAME}"; \
		chmod +x "$$package_dir/${BINARY_NAME}"; \
		cp ./README.md "$$package_dir/"; \
		cd "${BUILD_DIR}/tmp" && zip -r "../${BINARY_NAME}-v${VERSION}-linux-$$arch.zip" "ghoti-linux-$$arch" && cd - > /dev/null; \
	done
	@# macOS packages
	@for arch in amd64 arm64; do \
		echo "Creating package for darwin-$$arch..."; \
		package_dir="${BUILD_DIR}/tmp/ghoti-darwin-$$arch"; \
		mkdir -p "$$package_dir"; \
		cp "${BUILD_DIR}/${BINARY_NAME}-darwin-$$arch" "$$package_dir/${BINARY_NAME}"; \
		chmod +x "$$package_dir/${BINARY_NAME}"; \
		cp ./README.md "$$package_dir/"; \
		cd "${BUILD_DIR}/tmp" && zip -r "../${BINARY_NAME}-v${VERSION}-darwin-$$arch.zip" "ghoti-darwin-$$arch" && cd - > /dev/null; \
	done
	@# Windows packages
	@for arch in amd64 386; do \
		echo "Creating package for windows-$$arch..."; \
		package_dir="${BUILD_DIR}/tmp/ghoti-windows-$$arch"; \
		mkdir -p "$$package_dir"; \
		cp "${BUILD_DIR}/${BINARY_NAME}-windows-$$arch.exe" "$$package_dir/${BINARY_NAME}.exe"; \
		cp ./README.md "$$package_dir/"; \
		cd "${BUILD_DIR}/tmp" && zip -r "../${BINARY_NAME}-v${VERSION}-windows-$$arch.zip" "${BINARY_NAME}-windows-$$arch" && cd - > /dev/null; \
	done
	rm -rf "${BUILD_DIR}/tmp"
	@echo "Release packages are available in ${BUILD_DIR}"

#-----------------------------------------------------------------------------------------------------------------------
# Helpers
#-----------------------------------------------------------------------------------------------------------------------
define print
	@printf "${TEXT_ENABLE_STANDOUT}${COLOR_WHITE} 🚀 ${COLOR_BLUE} %-70s ${COLOR_WHITE} ${TEXT_DISABLE_STANDOUT}\n" $(1)
endef
