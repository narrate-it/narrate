package pocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderMissingRemoteSettingsMakesNoRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	_, err := Render(context.Background(), Config{SparkURL: server.URL}, "sample", t.TempDir(), &strings.Builder{})
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected UnavailableError, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("made %d requests with missing remote settings", requests)
	}
}

func TestPreflightRequiresGPUForCUDA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"available":{"cpuMillis":1000,"memoryMB":6144,"gpuCount":0}}`))
	}))
	defer server.Close()
	err := preflight(context.Background(), server.URL, "cuda", 3)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected GPU capacity to be unavailable, got %v", err)
	}
	if err := preflight(context.Background(), server.URL, "cpu", 3); err != nil {
		t.Fatalf("CPU preflight should pass: %v", err)
	}
}

func TestPreflightFailureIsTypedUnavailable(t *testing.T) {
	err := preflight(context.Background(), "http://127.0.0.1:1", "cuda", 3)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected UnavailableError, got %v", err)
	}
}
