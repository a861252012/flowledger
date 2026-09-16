#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
exec go run ./cmd/send-and-verify "$@"
