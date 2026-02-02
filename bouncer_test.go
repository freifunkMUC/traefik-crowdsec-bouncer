package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/decisions" {
			http.NotFound(w, r)
			return
		}
		ip := r.URL.Query().Get("ip")
		if ip == "127.0.0.1" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("null"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"type":"ban"}]`))
	}))

	u, _ := url.Parse(server.URL)
	_ = os.Setenv("CROWDSEC_BOUNCER_API_KEY", "test-api-key")
	_ = os.Setenv("CROWDSEC_AGENT_HOST", u.Host)
	_ = os.Setenv("CROWDSEC_BOUNCER_SCHEME", u.Scheme)
	_ = os.Setenv("CROWDSEC_BOUNCER_BAN_RESPONSE_CODE", "403")
	_ = os.Setenv("CROWDSEC_BOUNCER_BAN_RESPONSE_MSG", "Forbidden")

	code := m.Run()
	server.Close()
	os.Exit(code)
}

func TestPing(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/ping", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "pong", w.Body.String())
}
func TestHealthz(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/healthz", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}
func TestMetrics(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/metrics", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "go_info")
	assert.Contains(t, w.Body.String(), "crowdsec_traefik_bouncer_processed_ip_total")
}

func TestForwardAuthInvalidIp(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/forwardAuth", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Equal(t, "Forbidden", w.Body.String())
}
func TestForwardAuthBannedIp(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/forwardAuth", nil)
	req.RemoteAddr = "1.2.3.4:48328"
	router.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Equal(t, "Forbidden", w.Body.String())
}
func TestForwardAuthValidIp(t *testing.T) {
	router, _ := setupRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/forwardAuth", nil)
	req.RemoteAddr = "127.0.0.1:48328"
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}
