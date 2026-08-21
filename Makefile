# Thin bridge over Taskfile so `make` works for CI and habit.
# The real task definitions live in Taskfile.yaml.
#
# build/install mirror Taskfile.yaml's GIT_VERSION + ldflags exactly, so a
# binary built with `make` reports the same version as one built with `task`
# (krmgenVer / krmgenGenerated read this at runtime).

.PHONY: build test lint check install

GIT_VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(GIT_VERSION)

build:
	mkdir -p build
	go build -o build/krmgen -ldflags "$(LDFLAGS)" .

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

check: build test

install:
	GOBIN="$(HOME)/go/bin" go install -ldflags "$(LDFLAGS)" .
