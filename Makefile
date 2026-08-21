# Thin bridge over Taskfile so `make` works for CI and habit.
# The real task definitions live in Taskfile.yaml.

.PHONY: build test lint check install

build:
	go build -o build/krmgen .

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

check: build test

install:
	go install .
