#!/usr/bin/env bash
# Runs the image and checks it from the outside. There is no CrowdSec here, so
# the LAPI address points nowhere: the bouncer has to come up regardless, and
# has to refuse a request it cannot check rather than let it through.
set -euo pipefail

image=$1

docker run -d --name smoke -p 8080:8080 \
  -e CROWDSEC_BOUNCER_API_KEY=smoke-test \
  -e CROWDSEC_AGENT_HOST=127.0.0.1:1 \
  "$image"
trap 'docker logs smoke; docker rm -f smoke >/dev/null' EXIT
for _ in $(seq 30); do curl -sf http://localhost:8080/api/v1/ping >/dev/null && break; sleep 1; done

test "$(curl -sf http://localhost:8080/api/v1/ping)" = pong
grep -q '^crowdsec_traefik_bouncer_' <<< "$(curl -sf http://localhost:8080/api/v1/metrics)"
test "$(curl -s -o /dev/null -w '%{http_code}' -H 'X-Real-Ip: 192.0.2.1' http://localhost:8080/api/v1/forwardAuth)" = 403
