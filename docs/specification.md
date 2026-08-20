# krmgen specification

This document defines the contract of krmgen: what it accepts, what it produces,
and what guarantees hold. It is the reference against which implementation changes
are measured — if behaviour differs from this document, one of the two is a bug.

Version: applies to krmgen 0.x. Status: draft.

## 1. CLI contract

### Commands

| Command | Aliases | Description |
|---|---|---|
| `krmgen generate <path>` | `g` | Render KRM YAML from the config found in `<path>` |

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--skip` | string array, repeatable | none | Glob pattern of files copied without Go template evaluation |

### Streams

| Stream | Content |
|---|---|
| stdout | Rendered YAML, and nothing else |
| stderr | Log messages, warnings, errors |

Anything consuming krmgen may redirect stderr to `/dev/null` and still receive
valid YAML on stdout. This separation is a guarantee, not an implementation detail.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Rendering succeeded; YAML written to stdout |
| 1 | Any error: missing argument, unreadable path, template failure, helm or kustomize failure |

krmgen currently does not distinguish error classes by exit code. Callers must not
rely on a specific non-zero value beyond "non-zero means failure".

## 2. Configuration

To be completed in Task 2.

## 3. Rendering pipeline

To be completed in Task 3.

## 4. Template functions

To be completed in Task 4.

## 5. External tool support matrix

To be completed in Task 5.

## 6. Backend parity exceptions

To be completed in Task 6.

## 7. Non-goals

To be completed in Task 6.
