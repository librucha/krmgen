#!/bin/sh
# Generates shell completion scripts into completions/ for the release
# archives and the Homebrew cask (goreleaser `before` hook). The scripts come
# from cobra's built-in `krmgen completion <shell>` command.
set -e
rm -rf completions
mkdir completions
for sh in bash zsh fish; do
  go run . completion "$sh" > "completions/krmgen.$sh"
done
