package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testInfo() map[string]string {
	return map[string]string{
		"app":       "hello-k8s",
		"version":   "test",
		"pod":       "pod-1",
		"namespace": "ns-1",
		"node":      "node-1",
	}
}

// do issues a request against a fresh router and returns the recorder.
func do(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	newRouter(testInfo()).ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(t, http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("/healthz body = %q, want %q", got, "ok")
	}
}

func TestReadyz_WarmupThenReady(t *testing.T) {
	ready.Store(false)
	if rec := do(t, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz while warming = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	ready.Store(true)
	t.Cleanup(func() { ready.Store(false) })
	if rec := do(t, http.MethodGet, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("/readyz when ready = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRoot_IncrementsCounter(t *testing.T) {
	requests.Store(0)
	for i := 0; i < 3; i++ {
		if rec := do(t, http.MethodGet, "/"); rec.Code != http.StatusOK {
			t.Fatalf("/ request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("request counter = %d, want 3", got)
	}
}

func TestRoot_UnknownPathIs404(t *testing.T) {
	if rec := do(t, http.MethodGet, "/does-not-exist"); rec.Code != http.StatusNotFound {
		t.Fatalf("/does-not-exist status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMetrics_ExposesCounter(t *testing.T) {
	requests.Store(7)
	rec := do(t, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, "hello_requests_total 7") {
		t.Fatalf("/metrics body missing counter, got:\n%s", body)
	}
}

func TestVersion_ReturnsJSON(t *testing.T) {
	rec := do(t, http.MethodGet, "/version")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("/version content-type = %q, want application/json", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"version": "test"`) {
		t.Fatalf("/version body missing version field, got:\n%s", body)
	}
}
