package main

import (
	"os"

	"github.com/fbonalair/traefik-crowdsec-bouncer/config"
	"github.com/fbonalair/traefik-crowdsec-bouncer/controller"
	"github.com/gin-contrib/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"strings"
)

var logLevel = config.OptionalEnv("CROWDSEC_BOUNCER_LOG_LEVEL", "1")
var trustedProxiesList = strings.Split(config.OptionalEnv("TRUSTED_PROXIES", "0.0.0.0/0"), ",")

func main() {
	config.ValidateEnv()
	router, err := setupRouter()
	if err != nil {
		log.Fatal().Err(err).Msgf("An error occurred while starting webserver")
		return
	}

	err = router.Run()
	if err != nil {
		log.Fatal().Err(err).Msgf("An error occurred while starting bouncer")
		return
	}

}

func setupRouter() (*gin.Engine, error) {
	// logger framework
	if gin.IsDebugging() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(
			zerolog.ConsoleWriter{
				Out:        os.Stderr,
				NoColor:    false,
				TimeFormat: zerolog.TimeFieldFormat,
			},
		)
	}
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		return nil, err
	}
	zerolog.SetGlobalLevel(level)

	// Web framework
	router := gin.New()
	if err := router.SetTrustedProxies(trustedProxiesList); err != nil {
		return nil, err
	}
	router.Use(logger.SetLogger(
		logger.WithSkipPath([]string{"/api/v1/ping", "/api/v1/healthz"}),
	))
	router.GET("/api/v1/ping", controller.Ping)
	router.GET("/api/v1/healthz", controller.Healthz)
	router.GET("/api/v1/forwardAuth", controller.ForwardAuth)
	router.GET("/api/v1/metrics", controller.Metrics)
	return router, nil
}
