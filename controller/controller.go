package controller

import (
	"bytes"
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
	realIpHeader         = "X-Real-Ip"
	forwardHeader        = "X-Forwarded-For"
	crowdsecAuthHeader   = "X-Api-Key"
	crowdsecBouncerRoute = "v1/decisions"
	healthCheckIp        = "127.0.0.1"
)

type controllerConfig struct {
	apiKey          string
	host            string
	scheme          string
	banResponseCode int
	banResponseMsg  string
	banResponseFile string
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
		banResponseCode := config.OptionalEnv("CROWDSEC_BOUNCER_BAN_RESPONSE_CODE", "403")
		parsedCode, err := strconv.Atoi(banResponseCode)
		if err != nil {
			log.Fatal().Err(err).Msgf("The value for env var %s is not an int. It should be a valid http response code.", "CROWDSEC_BOUNCER_BAN_RESPONSE_CODE")
		}
		cfg.banResponseCode = parsedCode
	})

	return cfg
}
var (
	ipProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crowdsec_traefik_bouncer_processed_ip_total",
		Help: "The total number of processed IP",
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
func isIpAuthorized(clientIP string) (bool, error) {
	config := getConfig()
	// Generate Crowdsec API request
	decisionUrl := url.URL{
		Scheme:   config.scheme,
		Host:     config.host,
		Path:     crowdsecBouncerRoute,
		RawQuery: fmt.Sprintf("type=ban&ip=%s", clientIP),
	}
	req, err := http.NewRequest(http.MethodGet, decisionUrl.String(), nil)
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
Main route used by Traefik to verify authorization for a request
*/
func ForwardAuth(c *gin.Context) {
	ipProcessed.Inc()
	clientIP := c.ClientIP()

	log.Debug().
		Str("ClientIP", clientIP).
		Str("RemoteAddr", c.Request.RemoteAddr).
		Str(forwardHeader, c.Request.Header.Get(forwardHeader)).
		Str(realIpHeader, c.Request.Header.Get(realIpHeader)).
		Msg("Handling forwardAuth request")

	// Getting and verifying ip using ClientIP function
	isAuthorized, err := isIpAuthorized(clientIP)
	if err != nil {
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
	isHealthy, err := isIpAuthorized(healthCheckIp)
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
