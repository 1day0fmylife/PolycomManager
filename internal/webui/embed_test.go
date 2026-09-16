package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandlerRootDoesNotRedirect(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if location := rr.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect Location: %q", location)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, expected HTML", contentType)
	}
	if !strings.Contains(rr.Body.String(), "<html") && !strings.Contains(rr.Body.String(), "<!doctype html") {
		t.Fatalf("root did not return HTML")
	}
}

func TestSPAHandlerClientRouteFallsBackToIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/devices/example", nil)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if location := rr.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect Location: %q", location)
	}
	if !strings.Contains(rr.Body.String(), "<html") && !strings.Contains(rr.Body.String(), "<!doctype html") {
		t.Fatalf("SPA fallback did not return HTML")
	}
}

func TestSPAHandlerServesStaticAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/app.js", nil)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, expected JavaScript", got)
	}
}

func TestSPAHandlerMissingStaticAssetReturns404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/assets/missing.js", nil)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if !strings.Contains(rr.Body.String(), "web UI asset not found") {
		t.Fatalf("unexpected response body: %q", rr.Body.String())
	}
}

func TestSPAHandlerHeadRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodHead, "http://localhost/", nil)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
