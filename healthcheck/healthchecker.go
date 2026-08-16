package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

/**
  Simple binary to query bouncer health check route and allow use of docker container health check
  For more information, see issue https://github.com/fbonalair/traefik-crowdsec-bouncer/issues/6
*/
func main() {
	port := os.Getenv("PORT")
	timeoutString := os.Getenv("HEALTH_CHECKER_TIMEOUT_DURATION")
	if port == "" {
		port = "8080"
	}
	if timeoutString == "" {
		timeoutString = "2s"
	}

	// In stream mode, /healthz is a cheap local cache-freshness check (no
	// CrowdSec round-trip), so it's safe and more useful to use it here
	// instead of the plain liveness-only /ping. In live mode /healthz would
	// make a live LAPI call on every health check and restarting the
	// container wouldn't fix a CrowdSec outage anyway, so /ping stays the
	// default there.
	route := "/api/v1/ping"
	if os.Getenv("CROWDSEC_BOUNCER_STREAM_MODE") == "true" {
		route = "/api/v1/healthz"
	}

	// Calling bouncer health check
	healthCheckUrl := fmt.Sprintf("http://127.0.0.1:%s%s", port, route)
	duration, err := time.ParseDuration(timeoutString)
	if err != nil {
		log.Fatal("error while parsing HEALTH_CHECKER_TIMEOUT_DURATION value to duration: ", err)
	}
	httpClient := http.Client{Timeout: duration}
	resp, err := httpClient.Get(healthCheckUrl)
	if err != nil {
		log.Fatal("error while requesting bouncer's health check route: ", err)
	}

	if resp.StatusCode == http.StatusOK {
		os.Exit(0)
	}

	os.Exit(1)
}
