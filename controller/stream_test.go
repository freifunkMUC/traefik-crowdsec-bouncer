package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fbonalair/traefik-crowdsec-bouncer/cache"
	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
	"github.com/stretchr/testify/assert"
)

var (
	mockStreamMu   sync.Mutex
	mockStreamNew  []model.Decision
	mockStreamDel  []model.Decision
	mockStreamFail bool
	mockStreamHits int
)

func resetMockStream() {
	mockStreamMu.Lock()
	defer mockStreamMu.Unlock()
	mockStreamNew = nil
	mockStreamDel = nil
	mockStreamFail = false
	mockStreamHits = 0
}

// resetStreamState resets the package-level stream/cache state so each test
// starts from a clean slate, independent of test execution order.
func resetStreamState() {
	decisionCache = cache.New()
	streamMu.Lock()
	lastSyncSuccess = time.Time{}
	streamInitialized = false
	streamMu.Unlock()
	resetMockStream()
}

func TestMain(m *testing.M) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/decisions/stream" {
			http.NotFound(w, r)
			return
		}
		mockStreamMu.Lock()
		defer mockStreamMu.Unlock()
		mockStreamHits++
		if mockStreamFail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		body, _ := json.Marshal(streamResponse{New: mockStreamNew, Deleted: mockStreamDel})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))

	u, _ := url.Parse(server.URL)
	_ = os.Setenv("CROWDSEC_BOUNCER_API_KEY", "test-api-key")
	_ = os.Setenv("CROWDSEC_AGENT_HOST", u.Host)
	_ = os.Setenv("CROWDSEC_BOUNCER_SCHEME", u.Scheme)
	_ = os.Setenv("CROWDSEC_BOUNCER_STREAM_INTERVAL", "30ms")
	streamInitialRetryDelay = time.Millisecond

	code := m.Run()
	server.Close()
	os.Exit(code)
}

func TestSyncOnceAppliesNewDecisions(t *testing.T) {
	resetStreamState()
	mockStreamNew = []model.Decision{{Scope: "Ip", Value: "1.2.3.4"}}

	err := syncOnce(context.Background(), true)

	assert.NoError(t, err)
	assert.True(t, decisionCache.IsBanned("1.2.3.4"))
	assert.True(t, streamHealthy())
}

func TestSyncOnceReturnsErrorOnFailureAndLeavesCacheUntouched(t *testing.T) {
	resetStreamState()
	mockStreamNew = []model.Decision{{Scope: "Ip", Value: "1.2.3.4"}}
	require := assert.New(t)
	require.NoError(syncOnce(context.Background(), true))

	mockStreamMu.Lock()
	mockStreamFail = true
	mockStreamMu.Unlock()

	err := syncOnce(context.Background(), false)

	assert.Error(t, err)
	// Cache from the earlier successful sync must still be intact.
	assert.True(t, decisionCache.IsBanned("1.2.3.4"))
}

func TestStreamHealthyBeforeAnySync(t *testing.T) {
	resetStreamState()
	assert.False(t, streamHealthy())
}

func TestIsIpAuthorizedFromCacheFailsClosedWhenNotReady(t *testing.T) {
	resetStreamState()

	authorized, err := isIpAuthorizedFromCache("1.2.3.4")

	assert.Error(t, err)
	assert.False(t, authorized)
}

func TestIsIpAuthorizedFromCacheAfterSync(t *testing.T) {
	resetStreamState()
	mockStreamNew = []model.Decision{{Scope: "Ip", Value: "1.2.3.4"}}
	assert.NoError(t, syncOnce(context.Background(), true))

	bannedAuthorized, err := isIpAuthorizedFromCache("1.2.3.4")
	assert.NoError(t, err)
	assert.False(t, bannedAuthorized)

	cleanAuthorized, err := isIpAuthorizedFromCache("9.9.9.9")
	assert.NoError(t, err)
	assert.True(t, cleanAuthorized)
}

func TestStartStreamRetriesUntilInitialSyncSucceeds(t *testing.T) {
	resetStreamState()
	mockStreamMu.Lock()
	mockStreamFail = true
	mockStreamMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		WaitStreamStopped()
	}()

	done := make(chan error, 1)
	go func() { done <- StartStream(ctx) }()

	// Let a couple of failed attempts happen, then let it succeed.
	time.Sleep(5 * time.Millisecond)
	mockStreamMu.Lock()
	mockStreamFail = false
	mockStreamMu.Unlock()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("StartStream did not complete its initial sync in time")
	}
	assert.True(t, streamHealthy())
}

func TestStartStreamBackgroundLoopPicksUpIncrementalUpdates(t *testing.T) {
	resetStreamState()

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		WaitStreamStopped()
	}()

	assert.NoError(t, StartStream(ctx))
	assert.False(t, decisionCache.IsBanned("5.5.5.5"))

	mockStreamMu.Lock()
	mockStreamNew = []model.Decision{{Scope: "Ip", Value: "5.5.5.5"}}
	mockStreamMu.Unlock()

	assert.Eventually(t, func() bool {
		return decisionCache.IsBanned("5.5.5.5")
	}, time.Second, 5*time.Millisecond, "background sync did not pick up the new decision in time")
}

func TestStartStreamIfEnabledIsNoopByDefault(t *testing.T) {
	resetStreamState()
	// CROWDSEC_BOUNCER_STREAM_MODE isn't set in TestMain, so this must return
	// immediately without ever hitting the mock stream server.
	assert.NoError(t, StartStreamIfEnabled(context.Background()))
	assert.Equal(t, 0, mockStreamHits)
	assert.False(t, streamHealthy())
}
