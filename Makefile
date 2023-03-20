OUT_DIR ?= build

# The binary to build (just the basename).
BIN ?= krmgen

# This repo's root import path (under GOPATH).
PKG := github.com/librucha/krmgen

###
### These variables should not need tweaking.
###

# Which architecture to build - see $(ALL_ARCH) for options.
ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)

VERSION ?= $(shell cat version.txt)

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

CLI_PLATFORMS ?= darwin-amd64 darwin-arm64 freebsd-386 freebsd-amd64 freebsd-arm linux-386 linux-amd64 linux-arm linux-arm64 linux-mips linux-mips64 linux-mips64le linux-mipsle linux-ppc64 linux-ppc64le linux-s390x netbsd-386 netbsd-amd64 netbsd-arm openbsd-386 openbsd-amd64 windows-386 windows-amd64

.PHONY: build
build: $(OUT_DIR)/bin/$(GOOS)/$(GOARCH)/$(BIN)

dist: $(OUT_DIR)/dist/$(GOOS)/$(GOARCH)/$(BIN)

local-build: clean build-dirs
	@echo "building local"
	GOOS=$(shell go env GOOS) \
	GOARCH=$(shell go env GOARCH) \
	VERSION=$(VERSION) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	GIT_SHA=$(GIT_SHA) \
	GIT_TREE_STATE=$(GIT_TREE_STATE) \
	OUTPUT_DIR=$$(pwd)/$(OUT_DIR) \
	MAKE_CHECKSUMS=false \
	./hack/build.sh

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
	OUTPUT_DIR=$$(pwd)/$(OUT_DIR)/$(GOOS)/$(GOARCH) \
	MAKE_CHECKSUMS=false \
	./hack/build.sh

$(OUT_DIR)/dist/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
# Add DEBUG=1 to enable debug locally
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	VERSION=$(VERSION) \
	PKG=$(PKG) \
	BIN=$(BIN)_$(GOOS)_$(GOARCH) \
	GIT_SHA=$(GIT_SHA) \
	GIT_TREE_STATE=$(GIT_TREE_STATE) \
	OUTPUT_DIR=$$(pwd)/$(OUT_DIR) \
	MAKE_CHECKSUMS=true \
	./hack/build.sh

# Example: make shell CMD="date > datefile"
shell: build-dirs
	$(shell /bin/sh $(CMD))

.PHONY: clean
clean:
	rm -rf $(OUT_DIR)

.PHONY: all
all:
	@$(MAKE) clean all-dist

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

dist-%:
	@$(MAKE) --no-print-directory ARCH=$* dist

all-dist: $(addprefix dist-, $(CLI_PLATFORMS))

test: build-dirs
	@$(MAKE) shell CMD="-c 'hack/test.sh $(WHAT)'"

build-dirs:
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build .go/golangci-lint
