OUT_DIR ?= build

# The binary to build (just the basename).
BIN ?= krmgen

# This repo's root import path (under GOPATH).
PKG := github.com/librucha/krmgen

###
### These variables should not need tweaking.
###

# Which architecture to build - see $(ALL_ARCH) for options.
#ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

VERSION ?= main

# set git sha and tree state
GIT_SHA = $(shell git rev-parse HEAD)
ifneq ($(shell git status --porcelain 2> /dev/null),)
	GIT_TREE_STATE ?= dirty
else
	GIT_TREE_STATE ?= clean
endif

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))
GOPROXY ?= https://proxy.golang.org

CLI_PLATFORMS ?= linux-amd64 linux-arm linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 linux-ppc64le

.PHONY: build
build: $(OUT_DIR)/bin/$(GOOS)/$(GOARCH)/$(BIN)

$(OUT_DIR)/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
# Add DEBUG=1 to enable debug locally
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	VERSION=$(VERSION) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	GIT_SHA=$(GIT_SHA) \
	GIT_TREE_STATE=$(GIT_TREE_STATE) \
	OUTPUT_DIR=$$(pwd)/$(OUT_DIR)/bin/$(GOOS)/$(GOARCH) \
	./hack/build.sh

# Example: make shell CMD="date > datefile"
shell: build-dirs
	$(shell /bin/sh $(CMD))

.PHONY: clean
clean:
	rm -rf $(OUT_DIR)

.PHONY: all
all:
	@$(MAKE) clean all-build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

all-build: $(addprefix build-, $(CLI_PLATFORMS))

test: build-dirs
	@$(MAKE) shell CMD="-c 'hack/test.sh $(WHAT)'"

build-dirs:
	@mkdir -p $(OUT_DIR)/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build .go/golangci-lint
