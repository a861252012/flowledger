#!/bin/bash
# Root-owned timer: fetch only this repository's current main release.
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
unset DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_PROFILES
readonly base=/opt/testnet-wallet-lab
readonly repository=ghcr.io/a861252012/testnet-wallet-lab
exec 8>"$base/poll.lock"
flock -n 8 || exit 0
head=$(curl --fail --silent --show-error --max-time 20 -H 'Cache-Control: no-cache' https://api.github.com/repos/a861252012/testnet-wallet-lab/commits/main | python3 -c 'import json,sys; print(json.load(sys.stdin)["sha"])')
[[ "$head" =~ ^[a-f0-9]{40}$ ]] || exit 1
if [[ -f "$base/current-image" ]]; then
  current=$(cat "$base/current-image")
  [[ "$current" =~ ^ghcr.io/a861252012/testnet-wallet-lab@sha256:[a-f0-9]{64}$ ]] || exit 1
  revision=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$current")
  [[ "$revision" != "$head" ]] || exit 0
fi
# A failed/pending CI has no release tag. Pull failure never stops the old app.
docker pull "$repository:sha-$head"
ref=$(docker image inspect --format '{{index .RepoDigests 0}}' "$repository:sha-$head")
[[ "$ref" =~ ^ghcr.io/a861252012/testnet-wallet-lab@sha256:[a-f0-9]{64}$ ]] || exit 1
/usr/local/sbin/wallet-deploy "${ref#*@}" "$head"
