[← Back to README](../README.md)

# Dagger CI/CD Pipeline — `.dagger/`

## Overview

The project includes a [Dagger](https://dagger.io/) module for running containerised CI/CD functions. Dagger lets you define pipeline steps as Go functions that execute in containers, providing reproducible builds locally and in CI without writing shell scripts or YAML.

Currently the module ships two example functions (`ContainerEcho` and `GrepDir`) that demonstrate the Dagger SDK patterns. They serve as a starting point for adding project-specific build, test, and publish pipelines.

## Module Details

| Field | Value |
|---|---|
| Module name | `clock-server` |
| Engine version | `v0.19.11` |
| SDK | Go |
| Source directory | `.dagger/` |
| Go import path | `dagger/clock-server` |
| Config file | `dagger.json` (project root) |

The auto-generated SDK code lives in `.dagger/internal/dagger/dagger.gen.go` and should not be edited by hand.

## Pipeline Functions

### `ContainerEcho`

**Purpose:** Returns a container that echoes the provided string. Useful as a smoke test for the Dagger setup.

| Parameter | Type | Description |
|---|---|---|
| `stringArg` | `string` | The string to echo |

**What it does:** Creates an `alpine:latest` container and runs `echo <stringArg>`.

### `GrepDir`

**Purpose:** Searches files in a directory for lines matching a pattern.

| Parameter | Type | Description |
|---|---|---|
| `directoryArg` | `*dagger.Directory` | Directory to search |
| `pattern` | `string` | Grep pattern to match |

**What it does:** Mounts the provided directory at `/mnt` inside an `alpine:latest` container, then runs `grep -R <pattern> .` and returns the matching lines via stdout.

## Running Locally

Requires the [Dagger CLI](https://docs.dagger.io/install/) to be installed.

```bash
# Run the echo function
dagger call container-echo --string-arg "hello world"

# Grep for a pattern in the current directory
dagger call grep-dir --directory-arg . --pattern "TODO"
```

Dagger function names are converted to kebab-case on the CLI, and Go parameters become `--kebab-case` flags.
