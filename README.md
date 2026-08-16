![GitHub](https://img.shields.io/github/license/fbonalair/traefik-crowdsec-bouncer)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/fbonalair/traefik-crowdsec-bouncer)
[![Go Report Card](https://goreportcard.com/badge/github.com/fbonalair/traefik-crowdsec-bouncer)](https://goreportcard.com/report/github.com/fbonalair/traefik-crowdsec-bouncer)
[![Maintainability](https://api.codeclimate.com/v1/badges/7177dce30f0abdf8bcbf/maintainability)](https://codeclimate.com/github/fbonalair/traefik-crowdsec-bouncer/maintainability)
[![ci](https://github.com/freifunkMUC/traefik-crowdsec-bouncer/actions/workflows/build.yml/badge.svg)](https://github.com/freifunkMUC/traefik-crowdsec-bouncer/actions/workflows/build.yml)
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/fbonalair/traefik-crowdsec-bouncer)
![Docker Image Size (latest semver)](https://img.shields.io/docker/image-size/fbonalair/traefik-crowdsec-bouncer)

# traefik-crowdsec-bouncer

A HTTP service to verify requests and bounce them according to decisions made by CrowdSec.

# Description

This repository aims to implement a [CrowdSec](https://doc.crowdsec.net/) bouncer for the router [Traefik](https://doc.traefik.io/traefik/) to block malicious IPs from accessing your services. It leverages the [Traefik v2 ForwardAuth middleware](https://doc.traefik.io/traefik/middlewares/http/forwardauth/) and queries CrowdSec with the client IP. If the client IP is on the ban list, it will receive a HTTP code 403 response. Otherwise, the request will continue as usual.

# Demo

## Prerequisites

Ensure [Docker](https://docs.docker.com/get-docker/) and [Docker-compose](https://docs.docker.com/compose/install/) are installed. You can use the docker-compose file in the examples folder as a starting point. Through Traefik, it exposes the whoami container on port 80, with the bouncer accepting and rejecting client IPs.

Launch all services except the bouncer with the following commands:

```bash
git clone https://github.com/fbonalair/traefik-crowdsec-bouncer.git && \
  cd traefik-crowdsec-bouncer/examples && \
  docker-compose up -d traefik crowdsec whoami
```

## Procedure

1. Get a bouncer API key from CrowdSec with the command `docker exec crowdsec-example cscli bouncers add traefik-bouncer`
2. Copy the printed API key. You **_WON'T_** be able to retrieve it again.
3. Paste this API key as the value for the bouncer environment variable `CROWDSEC_BOUNCER_API_KEY`, instead of "MyApiKey"
4. Start the bouncer in attach mode with `docker-compose up bouncer`
5. Visit <http://localhost/>. You will see the container whoami page. Copy your IP address from the `X-Real-Ip` line (e.g., 192.168.128.1).  
   In your console, you will see lines showing your authorized request (i.e., "status": 200).
6. In another console, ban your IP with the command `docker exec crowdsec-example cscli decisions add --ip 192.168.128.1`, replacing the IP with your address.
7. Visit <http://localhost/> again. In your browser, you will see "Forbidden" since you have been banned.
   In the console, you will see "status": 403.
8. Unban yourself with `docker exec crowdsec-example cscli decisions delete --ip 192.168.128.1`
9. Visit <http://localhost/> one last time. You will have access to the container whoami.

Enjoy!

# Usage

For now, this web service is mainly intended to be used as a container. If you need to build from source, you can get some inspiration from the Dockerfile.

## Prerequisites

You should have Traefik v2/v3 and a CrowdSec instance running. The container is available as `ghcr.io/freifunkmuc/traefik-crowdsec-bouncer`. Host it as you see fit, though it must have access to CrowdSec and be accessible by Traefik. Follow the [Traefik ForwardAuth middleware](https://doc.traefik.io/traefik/middlewares/http/forwardauth/) documentation to create a forwardAuth middleware pointing to your bouncer host. Generate a bouncer API key following [CrowdSec documentation](https://doc.crowdsec.net/docs/cscli/cscli_bouncers_add).

Note that CrowdSec refuses to start in Docker/Podman without a persistent volume for `/var/lib/crowdsec/data/` (already set up in the compose files in this repo).

For a Kubernetes deployment (Traefik's Kubernetes CRD provider, readiness/liveness probes, a `NetworkPolicy` restricting the bouncer to only be reachable from Traefik), see [`k8s/`](k8s/).

## Configuration

The web service configuration is managed via environment variables:

- `CROWDSEC_BOUNCER_API_KEY` - CrowdSec bouncer API key required to authorize requests to the local API (required)
- `CROWDSEC_AGENT_HOST` - Host and port of the CrowdSec agent, e.g., crowdsec-agent:8080 (required)
- `CROWDSEC_BOUNCER_SCHEME` - Scheme to query the CrowdSec agent. Expected values: http, https. Defaults to http
- `CROWDSEC_BOUNCER_LOG_LEVEL` - Minimum log level for the bouncer. Expected values: [zerolog levels](https://pkg.go.dev/github.com/rs/zerolog#readme-leveled-logging). Defaults to 1
- `CROWDSEC_BOUNCER_BAN_RESPONSE_CODE` - HTTP code to respond in case of a ban. Defaults to 403
- `CROWDSEC_BOUNCER_BAN_RESPONSE_MSG` - HTTP body message to respond in case of a ban. Defaults to "Forbidden"
- `CROWDSEC_BOUNCER_BAN_RESPONSE_FILE` - HTTP-File to respond in case of a ban. file should be included via volume and the absolute path should be used.
- `CROWDSEC_BOUNCER_FORWARD_AUTH_SECRET` - Optional shared secret. When set, `/api/v1/forwardAuth` only accepts requests carrying a matching `?secret=` query parameter and rejects everything else (see [Security](#security) below). Defaults to empty, i.e. disabled.
- `CROWDSEC_BOUNCER_STREAM_MODE` - `true`/`false`. When enabled, the bouncer no longer calls the CrowdSec LAPI on every request; instead it keeps a local, periodically refreshed copy of all active ban decisions and checks against that (see [Stream Mode](#stream-mode) below). Defaults to `false`.
- `CROWDSEC_BOUNCER_STREAM_INTERVAL` - How often to refresh the local decision cache when stream mode is enabled. [Golang duration string](https://pkg.go.dev/time#ParseDuration). Defaults to `10s`.
- `HEALTH_CHECKER_TIMEOUT_DURATION` - [Golang string representation of a duration](https://pkg.go.dev/time#ParseDuration) to wait for the bouncer's answer before failing the health check. Defaults to 2s
- `PORT` - Change the listening port of the web server. Defaults to 8080
- `GIN_MODE` - By default, runs the app in "debug" mode. Set it to "release" in production
- `TRUSTED_PROXIES` - List of trusted proxies' IP addresses in CIDR format, delimited by commas. Defaults to `0.0.0.0/0`. **Read the [Security](#security) section before relying on the default.**

## Stream Mode

By default (`CROWDSEC_BOUNCER_STREAM_MODE=false`, "live mode"), every single request handled by `/api/v1/forwardAuth` makes a synchronous HTTP call to the CrowdSec LAPI. That's simple and always up to date, but it also means: extra latency on every request, load on CrowdSec proportional to your total traffic, and if CrowdSec is slow or briefly unreachable, requests can block for up to the client timeout (5s) before failing closed — under a bad enough CrowdSec hiccup, that's felt by everything behind Traefik at once.

With `CROWDSEC_BOUNCER_STREAM_MODE=true`, the bouncer instead:

1. On startup, fetches the full list of active decisions from CrowdSec's [decision stream](https://docs.crowdsec.net/docs/local_api/decision_stream) (`/v1/decisions/stream?startup=true`) and retries with backoff until it succeeds — the bouncer won't accept traffic until this initial sync has completed.
2. Every `CROWDSEC_BOUNCER_STREAM_INTERVAL` (default `10s`), fetches only what changed (`startup=false`) and applies it to the local cache.
3. Answers every `/api/v1/forwardAuth` request from that local, in-memory cache — no network call, no per-request CrowdSec load.

Trade-offs to be aware of:

- **Bans/unbans take up to `CROWDSEC_BOUNCER_STREAM_INTERVAL` to take effect**, since they're only picked up on the next background sync — instant in live mode, eventually-consistent in stream mode.
- **The bouncer keeps serving from its last known-good cache for a while if CrowdSec becomes unreachable**, rather than immediately failing closed like live mode does. It only starts failing closed once the cache is stale (no successful sync for longer than 3× the sync interval) — this is what makes stream mode more resilient to brief CrowdSec hiccups than live mode.
- `/api/v1/healthz` reflects the stream cache's freshness in this mode (not a live LAPI call), and `crowdsec_traefik_bouncer_stream_sync_error_total` / `crowdsec_traefik_bouncer_stream_cached_decisions` (see [Exposed Routes](#exposed-routes)) are the metrics to alert on.
- The container's own `HEALTHCHECK` automatically switches from `/api/v1/ping` (live mode) to `/api/v1/healthz` (stream mode) based on `CROWDSEC_BOUNCER_STREAM_MODE` — in stream mode that check is a cheap local cache-freshness check, so Docker/Kubernetes will correctly mark the container unhealthy if the decision cache goes stale.

## Security

`TRUSTED_PROXIES` defaults to `0.0.0.0/0`, meaning the bouncer trusts the `X-Forwarded-For`/`X-Real-Ip` header from *any* source by default. This is necessary for `/api/v1/forwardAuth` to see the real client IP when called by Traefik, but it also means: **anyone who can reach the bouncer's port directly can set an arbitrary `X-Forwarded-For` header and bypass every CrowdSec ban.** The bouncer itself cannot fully close this gap, since Traefik's own IP is not knowable in advance across all deployment types (Docker, Kubernetes, bare metal, ...).

Because of this:

1. **Never expose the bouncer's port beyond Traefik.** It must not be reachable from the public internet, from other tenants in a shared cluster, or from any container/service that isn't Traefik itself.
2. Where possible, narrow `TRUSTED_PROXIES` to Traefik's actual IP/CIDR instead of the default.
3. As defense in depth, set `CROWDSEC_BOUNCER_FORWARD_AUTH_SECRET` to a random value and append it to Traefik's forwardAuth address, e.g.:

   ```yaml
   - "traefik.http.middlewares.crowdsec-bouncer.forwardauth.address=http://bouncer:8080/api/v1/forwardAuth?secret=<random-value>"
   ```

   Requests without the correct secret are rejected before the CrowdSec decision lookup even happens, so a spoofed `X-Forwarded-For` alone is no longer enough to bypass a ban if the bouncer's port is accidentally reachable.

`/api/v1/metrics` has the same exposure concern as `/api/v1/forwardAuth`: it isn't authenticated, so it shouldn't be reachable beyond whatever scrapes it (e.g. Prometheus) either.

## Exposed Routes

The web service exposes the following routes:

- GET `/api/v1/forwardAuth` - Main route to be used by Traefik: queries the CrowdSec agent with the header `X-Real-Ip` as the client IP
- GET `/api/v1/ping` - Simple health route that responds with "pong" and HTTP 200
- GET `/api/v1/healthz` - Another health route that queries the CrowdSec agent with localhost (127.0.0.1)
- GET `/api/v1/metrics` - Prometheus route to scrape metrics. Exposes `crowdsec_traefik_bouncer_processed_ip_total` and `crowdsec_traefik_bouncer_lookup_error_total` (requests denied because the CrowdSec lookup itself failed, e.g. LAPI unreachable — distinct from an actual ban, useful for alerting on fail-closed outages). In [stream mode](#stream-mode), also exposes `crowdsec_traefik_bouncer_stream_sync_error_total` and `crowdsec_traefik_bouncer_stream_cached_decisions{scope="ip"|"range"}`

# Contribution

Any constructive feedback is welcome. Feel free to add an issue or a pull request. I will review it and integrate it into the code.

## Local Setup

1. Start docker-compose with `docker-compose up -d`
2. Create `_test.env` from the template `_test.env.example` with the command `cp _test.env.example _test.env`
3. Get an API key for your bouncer with the command `docker exec traefik-crowdsec-bouncer-crowdsec-1 cscli bouncers add traefik-bouncer`
4. In `_test.env`, replace `<your_generated_api_key>` with the previously generated key
5. Add a banned IP to your CrowdSec instance with the command `docker exec traefik-crowdsec-bouncer-crowdsec-1 cscli decisions add -i 1.2.3.4`
6. Run tests with `godotenv -f ./_test.env go test -cover`
