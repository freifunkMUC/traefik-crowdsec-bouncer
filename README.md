
![GitHub](https://img.shields.io/github/license/fbonalair/traefik-crowdsec-bouncer)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/fbonalair/traefik-crowdsec-bouncer)
[![Go Report Card](https://goreportcard.com/badge/github.com/fbonalair/traefik-crowdsec-bouncer)](https://goreportcard.com/report/github.com/fbonalair/traefik-crowdsec-bouncer)
[![Maintainability](https://api.codeclimate.com/v1/badges/7177dce30f0abdf8bcbf/maintainability)](https://codeclimate.com/github/fbonalair/traefik-crowdsec-bouncer/maintainability)
[![ci](https://github.com/fbonalair/traefik-crowdsec-bouncer/actions/workflows/main.yml/badge.svg)](https://github.com/fbonalair/traefik-crowdsec-bouncer/actions/workflows/main.yml)
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

You should have Traefik v2 and a CrowdSec instance running. The container is available on Docker as the image `fbonalair/traefik-crowdsec-bouncer`. Host it as you see fit, though it must have access to CrowdSec and be accessible by Traefik. Follow the [Traefik v2 ForwardAuth middleware](https://doc.traefik.io/traefik/middlewares/http/forwardauth/) documentation to create a forwardAuth middle pointing to your bouncer host. Generate a bouncer API key following [CrowdSec documentation](https://doc.crowdsec.net/docs/cscli/cscli_bouncers_add).

## Configuration

The web service configuration is managed via environment variables:

* `CROWDSEC_BOUNCER_API_KEY`            - CrowdSec bouncer API key required to authorize requests to the local API (required)
* `CROWDSEC_AGENT_HOST`                 - Host and port of the CrowdSec agent, e.g., crowdsec-agent:8080 (required)
* `CROWDSEC_BOUNCER_SCHEME`             - Scheme to query the CrowdSec agent. Expected values: http, https. Defaults to http
* `CROWDSEC_BOUNCER_LOG_LEVEL`          - Minimum log level for the bouncer. Expected values: [zerolog levels](https://pkg.go.dev/github.com/rs/zerolog#readme-leveled-logging). Defaults to 1
* `CROWDSEC_BOUNCER_BAN_RESPONSE_CODE`  - HTTP code to respond in case of a ban. Defaults to 403
* `CROWDSEC_BOUNCER_BAN_RESPONSE_MSG`   - HTTP body message to respond in case of a ban. Defaults to "Forbidden"
* `HEALTH_CHECKER_TIMEOUT_DURATION`     - [Golang string representation of a duration](https://pkg.go.dev/time#ParseDuration) to wait for the bouncer's answer before failing the health check. Defaults to 2s
* `PORT`                                - Change the listening port of the web server. Defaults to 8080
* `GIN_MODE`                            - By default, runs the app in "debug" mode. Set it to "release" in production
* `TRUSTED_PROXIES`                     - List of trusted proxies' IP addresses in CIDR format, delimited by commas. Default is 0.0.0.0/0, which should be fine for most use cases, but you MUST add them directly in Traefik. 

## Exposed Routes

The web service exposes the following routes:

* GET `/api/v1/forwardAuth`             - Main route to be used by Traefik: queries the CrowdSec agent with the header `X-Real-Ip` as the client IP
* GET `/api/v1/ping`                    - Simple health route that responds with "pong" and HTTP 200
* GET `/api/v1/healthz`                 - Another health route that queries the CrowdSec agent with localhost (127.0.0.1)
* GET `/api/v1/metrics`                 - Prometheus route to scrape metrics

# Contribution

Any constructive feedback is welcome. Feel free to add an issue or a pull request. I will review it and integrate it into the code.    

## Local Setup 

1. Start docker-compose with `docker-compose up -d`
2. Create `_test.env` from the template `_test.env.example` with the command `cp _test.env.example _test.env`
3. Get an API key for your bouncer with the command `docker exec traefik-crowdsec-bouncer-crowdsec-1 cscli bouncers add traefik-bouncer`
4. In `_test.env`, replace `<your_generated_api_key>` with the previously generated key
5. Add a banned IP to your CrowdSec instance with the command `docker exec traefik-crowdsec-bouncer-crowdsec-1 cscli decisions add -i 1.2.3.4`
6. Run tests with `godotenv -f ./_test.env go test -cover`
