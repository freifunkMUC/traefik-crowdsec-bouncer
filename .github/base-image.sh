#!/usr/bin/env bash
# Prints the base images of a Dockerfile as key=value lines:
#   base_name=<image of the final stage, without digest>
#   base_digest=<its digest, empty if it is not pinned>
#   build_digests=<digests of the other stages, sorted, comma separated>
# The build and the daily rebuild put them into the image labels.
set -euo pipefail

dockerfile=${1:-Dockerfile}

refs=$(awk 'toupper($1) == "FROM" { for (i = 2; i <= NF; i++) if ($i !~ /^--/) { print $i; break } }' "$dockerfile")
[ -n "$refs" ] || { echo "No FROM in $dockerfile" >&2; exit 1; }

final=$(tail -n1 <<< "$refs")
echo "base_name=${final%@*}"
case "$final" in *@sha256:*) echo "base_digest=${final##*@}" ;; *) echo "base_digest=" ;; esac
echo "build_digests=$(head -n -1 <<< "$refs" | grep -o 'sha256:[0-9a-f]*' | sort -u | paste -sd, - || true)"
