package controller

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/fbonalair/traefik-crowdsec-bouncer/config"
	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const (
	realIpHeader           = "X-Real-Ip"
	forwardHeader          = "X-Forwarded-For"
	crowdsecAuthHeader     = "X-Api-Key"
	crowdsecBouncerRoute   = "v1/decisions"
	healthCheckIp          = "127.0.0.1"
	forwardAuthSecretParam = "secret"
)

type controllerConfig struct {
	apiKey            string
	host              string
	scheme            string
	banResponseCode   int
	banResponseMsg    string
	banResponseFile   string
	forwardAuthSecret string
	streamMode        bool
	streamInterval    time.Duration
}

var cfg controllerConfig
var cfgOnce sync.Once

func getConfig() controllerConfig {
	cfgOnce.Do(func() {
		cfg.apiKey = config.RequiredEnv("CROWDSEC_BOUNCER_API_KEY")
		cfg.host = config.RequiredEnv("CROWDSEC_AGENT_HOST")
		cfg.scheme = config.OptionalEnv("CROWDSEC_BOUNCER_SCHEME", "http")
		cfg.banResponseMsg = config.OptionalEnv("CROWDSEC_BOUNCER_BAN_RESPONSE_MSG", "Forbidden")
		cfg.banResponseFile = config.OptionalEnv("CROWDSEC_BOUNCER_BAN_RESPONSE_FILE", "")
		cfg.forwardAuthSecret = config.OptionalEnv("CROWDSEC_BOUNCER_FORWARD_AUTH_SECRET", "")
		banResponseCode := config.OptionalEnv("CROWDSEC_BOUNCER_BAN_RESPONSE_CODE", "403")
		parsedCode, err := strconv.Atoi(banResponseCode)
		if err != nil {
			log.Fatal().Err(err).Msgf("The value for env var %s is not an int. It should be a valid http response code.", "CROWDSEC_BOUNCER_BAN_RESPONSE_CODE")
		}
		cfg.banResponseCode = parsedCode

		streamMode := config.OptionalEnv("CROWDSEC_BOUNCER_STREAM_MODE", "false")
		parsedStreamMode, err := strconv.ParseBool(streamMode)
		if err != nil {
			log.Fatal().Err(err).Msgf("The value for env var %s is not a bool.", "CROWDSEC_BOUNCER_STREAM_MODE")
		}
		cfg.streamMode = parsedStreamMode

		streamInterval := config.OptionalEnv("CROWDSEC_BOUNCER_STREAM_INTERVAL", "10s")
		parsedStreamInterval, err := time.ParseDuration(streamInterval)
		if err != nil {
			log.Fatal().Err(err).Msgf("The value for env var %s is not a valid duration.", "CROWDSEC_BOUNCER_STREAM_INTERVAL")
		}
		cfg.streamInterval = parsedStreamInterval
	})

	return cfg
}

var (
	ipProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crowdsec_traefik_bouncer_processed_ip_total",
		Help: "The total number of processed IP",
	})
	lookupErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crowdsec_traefik_bouncer_lookup_error_total",
		Help: "The total number of forwardAuth requests that were denied because the CrowdSec decision lookup itself failed (e.g. LAPI unreachable), as opposed to an actual ban decision",
	})
)

var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	},
	Timeout: 5 * time.Second,
}

/*
*
Check whether HTML output is desired and include the HTML file
*/
func handleBanResponse(c *gin.Context) {
	config := getConfig()
	if config.banResponseFile != "" {
		if fileContent, err := os.ReadFile(config.banResponseFile); err == nil {
			if strings.HasSuffix(config.banResponseFile, ".html") {
				c.Data(http.StatusForbidden, "text/html", fileContent)
				return
			}
		}
	}
	// Fallback
	c.String(config.banResponseCode, config.banResponseMsg)
}

/*
*
Call Crowdsec local IP and with realIP and return true if IP does NOT have a ban decisions.
*/
func isIpAuthorized(ctx context.Context, clientIP string) (bool, error) {
	config := getConfig()
	// Generate Crowdsec API request
	decisionUrl := url.URL{
		Scheme:   config.scheme,
		Host:     config.host,
		Path:     crowdsecBouncerRoute,
		RawQuery: fmt.Sprintf("type=ban&ip=%s", clientIP),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, decisionUrl.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Add(crowdsecAuthHeader, config.apiKey)
	log.Debug().
		Str("method", http.MethodGet).
		Str("url", decisionUrl.String()).
		Msg("Requesting Crowdsec's decision Local API")

	// Call Crowdsec API
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("error closing response body: %v", cerr)
		}
	}()

	if resp.StatusCode == http.StatusForbidden {
		return false, nil
	}

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if bytes.Equal(respBody, []byte("null")) {
		log.Debug().Msgf("No decision for IP %q. Accepting", clientIP)
		return true, nil
	}

	log.Debug().RawJSON("decisions", respBody).Msg("Found Crowdsec's decision(s), evaluating ...")
	var decisions []model.Decision
	err = json.Unmarshal(respBody, &decisions)
	if err != nil {
		return false, err
	}

	// Authorization logic
	return len(decisions) == 0, nil
}

/*
Compare the "secret" query parameter (set as part of Traefik's static forwardAuth
address, e.g. http://bouncer:8080/api/v1/forwardAuth?secret=xxx) against the configured
CROWDSEC_BOUNCER_FORWARD_AUTH_SECRET. Only relevant when that env var is set; returns
true unconditionally otherwise, keeping the feature opt-in and backward compatible.
*/
func isForwardAuthSecretValid(c *gin.Context) bool {
	expected := getConfig().forwardAuthSecret
	if expected == "" {
		return true
	}
	provided := c.Query(forwardAuthSecretParam)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

/*
Main route used by Traefik to verify authorization for a request
*/
func ForwardAuth(c *gin.Context) {
	ipProcessed.Inc()
	clientIP := c.ClientIP()

	if !isForwardAuthSecretValid(c) {
		log.Warn().Str("ClientIP", clientIP).Msg("Rejecting forwardAuth request with missing or invalid secret")
		handleBanResponse(c)
		return
	}

	log.Debug().
		Str("ClientIP", clientIP).
		Str("RemoteAddr", c.Request.RemoteAddr).
		Str(forwardHeader, c.Request.Header.Get(forwardHeader)).
		Str(realIpHeader, c.Request.Header.Get(realIpHeader)).
		Msg("Handling forwardAuth request")

	// Getting and verifying ip, either against the local decision stream cache
	// or with a live CrowdSec LAPI call, depending on CROWDSEC_BOUNCER_STREAM_MODE
	var isAuthorized bool
	var err error
	if getConfig().streamMode {
		isAuthorized, err = isIpAuthorizedFromCache(clientIP)
	} else {
		isAuthorized, err = isIpAuthorized(c.Request.Context(), clientIP)
	}
	if err != nil {
		lookupErrors.Inc()
		log.Warn().Err(err).Msgf("An error occurred while checking IP %q", clientIP)
		handleBanResponse(c)
	} else if !isAuthorized {
		handleBanResponse(c)
	} else {
		c.Status(http.StatusOK)
	}
}

/*
Route to check bouncer connectivity with Crowdsec agent. Mainly use for Kubernetes readiness probe
*/
func Healthz(c *gin.Context) {
	if getConfig().streamMode {
		if !streamHealthy() {
			log.Warn().Msg("The health check did not pass: the CrowdSec decision stream cache is not initialized yet or stale")
			c.Status(http.StatusForbidden)
			return
		}
		c.Status(http.StatusOK)
		return
	}

	isHealthy, err := isIpAuthorized(c.Request.Context(), healthCheckIp)
	if err != nil || !isHealthy {
		log.Warn().Err(err).Msgf("The health check did not pass. Check error if present and if the IP %q is authorized", healthCheckIp)
		c.Status(http.StatusForbidden)
	} else {
		c.Status(http.StatusOK)
	}
}

/*
Simple route responding pong to every request. Mainly use for Kubernetes liveliness probe
*/
func Ping(c *gin.Context) {
	c.String(http.StatusOK, "pong")
}

func Metrics(c *gin.Context) {
	handler := promhttp.Handler()
	handler.ServeHTTP(c.Writer, c.Request)
}
