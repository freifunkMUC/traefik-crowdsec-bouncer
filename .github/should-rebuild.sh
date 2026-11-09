#!/usr/bin/env bash
# Decides whether the daily rebuild publishes; prints push=true or push=false.
#   should-rebuild.sh <published image:tag> <base digest> [<build stage digests>]
# The final image is distroless and has no package manager, so there is no
# list of pending upgrades to look at. The image is stale when it was built
# from other base images (final stage or Go build stage) than the Dockerfile
# pins now. FORCE=true publishes regardless.
set -euo pipefail

image=$1 base_digest=$2 build_digests=${3:-}

if [ "${FORCE:-false}" = true ]; then
  echo "Forced" >&2
  echo "push=true"
  exit 0
fi

if ! out=$(docker pull --quiet --platform linux/amd64 "$image" 2>&1); then
  if grep -qiE 'not found|manifest unknown' <<< "$out"; then
    echo "Nothing published yet" >&2
    echo "push=true"
    exit 0
  fi
  echo "Cannot pull $image: $out" >&2
  exit 1
fi

label() { docker inspect --format "{{ index .Config.Labels \"$1\" }}" "$image"; }
built_from=$(label org.opencontainers.image.base.digest)
built_with=$(label io.github.freifunkmuc.build-base-digests)

echo "Base digest: published ${built_from:-none}, Dockerfile $base_digest" >&2
echo "Build digests: published ${built_with:-none}, Dockerfile ${build_digests:-none}" >&2
if [ "$built_from" != "$base_digest" ] || [ "$built_with" != "$build_digests" ]; then
  echo "push=true"
else
  echo "up to date" >&2
  echo "push=false"
fi
