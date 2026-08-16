package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"

	"github.com/fbonalair/traefik-crowdsec-bouncer/cache"
	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
)

const crowdsecStreamRoute = "v1/decisions/stream"

// decisionCache holds the local copy of CrowdSec's active ban decisions when
// CROWDSEC_BOUNCER_STREAM_MODE is enabled.
var decisionCache = cache.New()

var (
	streamSyncErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crowdsec_traefik_bouncer_stream_sync_error_total",
		Help: "The total number of failed CrowdSec decision stream syncs",
	})
	streamCachedDecisions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "crowdsec_traefik_bouncer_stream_cached_decisions",
		Help: "The number of decisions currently held in the local stream cache, by scope",
	}, []string{"scope"})
)

// streamInitialRetryDelay is the delay before the first retry of a failed
// initial sync; it doubles on each subsequent attempt, capped at 30s. It's a
// var (not a const) so tests can shrink it.
var streamInitialRetryDelay = time.Second

var (
	streamMu          sync.RWMutex
	lastSyncSuccess   time.Time
	streamInitialized bool
)

type streamResponse struct {
	New     []model.Decision `json:"new"`
	Deleted []model.Decision `json:"deleted"`
}

func fetchStream(ctx context.Context, startup bool) (*streamResponse, error) {
	config := getConfig()
	streamUrl := url.URL{
		Scheme:   config.scheme,
		Host:     config.host,
		Path:     crowdsecStreamRoute,
		RawQuery: fmt.Sprintf("startup=%t", startup),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamUrl.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(crowdsecAuthHeader, config.apiKey)
	log.Debug().
		Str("method", http.MethodGet).
		Str("url", streamUrl.String()).
		Msg("Requesting Crowdsec's decision stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("error closing response body: %v", cerr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from CrowdSec decision stream: %s", resp.StatusCode, string(body))
	}

	var parsed streamResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func syncOnce(ctx context.Context, startup bool) error {
	resp, err := fetchStream(ctx, startup)
	if err != nil {
		return err
	}
	decisionCache.Apply(resp.New, resp.Deleted)

	ipCount, rangeCount := decisionCache.Size()
	streamCachedDecisions.WithLabelValues("ip").Set(float64(ipCount))
	streamCachedDecisions.WithLabelValues("range").Set(float64(rangeCount))

	streamMu.Lock()
	lastSyncSuccess = time.Now()
	if startup {
		streamInitialized = true
	}
	streamMu.Unlock()

	log.Debug().
		Int("new", len(resp.New)).
		Int("deleted", len(resp.Deleted)).
		Int("cachedIps", ipCount).
		Int("cachedRanges", rangeCount).
		Bool("startup", startup).
		Msg("Synced CrowdSec decision stream")
	return nil
}

// streamWG tracks the background sync goroutine(s) started by StartStream,
// so callers (tests, and any future graceful shutdown) can wait for it to
// have fully stopped after cancelling its context, rather than racing with
// it.
var streamWG sync.WaitGroup

// StartStream performs the initial full sync, retrying with capped
// exponential backoff until it succeeds or ctx is done, then keeps the local
// decision cache updated in the background (on config.streamInterval) until
// ctx is cancelled. It returns once the initial sync has succeeded, or ctx is
// done first, whichever comes first. Once ctx is cancelled, callers can wait
// for the background goroutine to fully stop via WaitStreamStopped.
func StartStream(ctx context.Context) error {
	config := getConfig()

	delay := streamInitialRetryDelay
	for {
		err := syncOnce(ctx, true)
		if err == nil {
			break
		}
		streamSyncErrors.Inc()
		log.Warn().Err(err).Msg("Initial CrowdSec decision stream sync failed, retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}

	streamWG.Add(1)
	go func() {
		defer streamWG.Done()
		ticker := time.NewTicker(config.streamInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := syncOnce(ctx, false); err != nil {
					streamSyncErrors.Inc()
					log.Warn().Err(err).Msg("CrowdSec decision stream sync failed, will retry next tick")
				}
			}
		}
	}()

	return nil
}

// WaitStreamStopped blocks until every background sync goroutine started by
// StartStream has returned. Intended to be called after cancelling the
// context passed to StartStream.
func WaitStreamStopped() {
	streamWG.Wait()
}

// StartStreamIfEnabled starts the CrowdSec decision stream sync when
// CROWDSEC_BOUNCER_STREAM_MODE is enabled; it is a no-op otherwise.
func StartStreamIfEnabled(ctx context.Context) error {
	if !getConfig().streamMode {
		return nil
	}
	return StartStream(ctx)
}

// isIpAuthorizedFromCache answers a forwardAuth check against the local
// decision cache instead of a live LAPI call. It returns an error (causing
// the caller to fail closed, same as a live lookup error) when the cache
// isn't ready or is stale, since serving from a cache that never
// successfully synced would silently let everyone through.
func isIpAuthorizedFromCache(clientIP string) (bool, error) {
	if !streamHealthy() {
		return false, fmt.Errorf("CrowdSec decision stream cache is not initialized yet or stale")
	}
	return !decisionCache.IsBanned(clientIP), nil
}

// streamHealthy reports whether the stream cache has completed its initial
// sync and was refreshed recently enough to be trusted.
func streamHealthy() bool {
	config := getConfig()
	streamMu.RLock()
	defer streamMu.RUnlock()
	if !streamInitialized {
		return false
	}
	return time.Since(lastSyncSuccess) < config.streamInterval*3
}
