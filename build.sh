#!/bin/bash

# --- Get dynamic values from Git and Date ---

# Get the current branch name
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# Get the latest tag with distance (e.g., v1.2.0-3-g123abc)
# If no tags exist, it falls back to the short commit hash.
GIT_TAG=$(git describe --tags --always --dirty)

# Get the full commit hash
GIT_COMMIT=$(git rev-parse HEAD)

# Get the current date/time in UTC
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# --- Define the Go Module Path ---
# Replace 'yourmodule/buildinfo' with your actual module path/package
MODULE_PATH="github.com/PAARA-org/PAARAbot/buildinfo"

# --- Construct the LDFLAGS string ---
LDFLAGS="-X ${MODULE_PATH}.GitCommit=${GIT_COMMIT}"
LDFLAGS="${LDFLAGS} -X ${MODULE_PATH}.GitBranch=${GIT_BRANCH}"
LDFLAGS="${LDFLAGS} -X ${MODULE_PATH}.GitTag=${GIT_TAG}"
LDFLAGS="${LDFLAGS} -X ${MODULE_PATH}.BuildDate=${BUILD_DATE}"

# --- Build the application ---
echo "Building with LDFLAGS: ${LDFLAGS}"
go build -ldflags "${LDFLAGS}" .
