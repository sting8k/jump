package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/sting8k/jump/services/jumpd/internal/config"
	"github.com/sting8k/jump/services/jumpd/internal/webprefs"
)

func registerFrontendConfigRoutes(mux *http.ServeMux, prefsMgr *webprefs.Manager) {
	// Frontend config combines user-editable config files with jumpd-managed
	// browser preferences. Config files are read on each request so users can
	// edit and refresh without restarting jumpd.
	mux.HandleFunc("GET /v1/frontend-config", func(w http.ResponseWriter, r *http.Request) {
		theme, themeErr := config.LoadTheme()
		settings, settingsErr := config.LoadSettings()
		prefs, prefsErr := prefsMgr.Load()
		if themeErr != nil {
			log.Printf("frontend-config: theme: %v", themeErr)
		}
		if settingsErr != nil {
			log.Printf("frontend-config: settings: %v", settingsErr)
		}
		if prefsErr != nil {
			log.Printf("frontend-config: preferences: %v", prefsErr)
			prefs = webprefs.DefaultState()
		}
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"theme":         theme,
				"settings":      settings,
				"appearance":    prefs.Appearance,
				"notifications": prefs.Notifications,
			},
		})
	})

	mux.HandleFunc("PATCH /v1/frontend-preferences", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 2048))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var req struct {
			Appearance    *webprefs.Appearance    `json:"appearance"`
			Notifications *webprefs.Notifications `json:"notifications"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}
		if req.Appearance == nil && req.Notifications == nil {
			writeError(w, http.StatusBadRequest, "bad_request", "preference patch required")
			return
		}

		prefs, err := prefsMgr.Update(req.Appearance, req.Notifications)
		if err != nil {
			if errors.Is(err, webprefs.ErrInvalidThemeID) {
				writeError(w, http.StatusBadRequest, "validation_error", err.Error())
				return
			}
			log.Printf("frontend-preferences: save error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to save preferences")
			return
		}
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"appearance":    prefs.Appearance,
				"notifications": prefs.Notifications,
			},
		})
	})
}
