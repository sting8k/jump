package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sting8k/jump/services/jumpd/internal/webprefs"
)

func TestFrontendConfigRoutesExposeDefaultAppearance(t *testing.T) {
	mux := http.NewServeMux()
	registerFrontendConfigRoutes(mux, webprefs.NewManager(t.TempDir()))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/frontend-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"appearance":{"theme_id":"default"}`) {
		t.Fatalf("response %q missing default appearance", body)
	}
}

func TestFrontendPreferencesPatchSavesAppearance(t *testing.T) {
	dir := t.TempDir()
	mux := http.NewServeMux()
	registerFrontendConfigRoutes(mux, webprefs.NewManager(dir))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/frontend-preferences", strings.NewReader(`{"appearance":{"theme_id":"slate-noir"}}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}

	state, err := webprefs.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Appearance.ThemeID != webprefs.SlateNoirThemeID {
		t.Fatalf("theme_id = %q, want %q", state.Appearance.ThemeID, webprefs.SlateNoirThemeID)
	}
}

func TestFrontendPreferencesPatchRejectsUnknownTheme(t *testing.T) {
	mux := http.NewServeMux()
	registerFrontendConfigRoutes(mux, webprefs.NewManager(t.TempDir()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/frontend-preferences", strings.NewReader(`{"appearance":{"theme_id":"unknown"}}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "validation_error") {
		t.Fatalf("response = %q, want validation_error", rec.Body.String())
	}
}
