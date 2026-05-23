package webprefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	state, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != currentVersion {
		t.Fatalf("version = %d, want %d", state.Version, currentVersion)
	}
	if state.Appearance.ThemeID != DefaultThemeID {
		t.Fatalf("theme_id = %q, want %q", state.Appearance.ThemeID, DefaultThemeID)
	}
	if state.Notifications.InApp || state.Notifications.OS {
		t.Fatalf("notifications = %+v, want all off", state.Notifications)
	}
	if state.Notifications.Ntfy.ServerURL != DefaultNtfyServerURL {
		t.Fatalf("ntfy server_url = %q, want %q", state.Notifications.Ntfy.ServerURL, DefaultNtfyServerURL)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	state := DefaultState()
	if err := state.Save(dir); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 600", perm)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Appearance.ThemeID != DefaultThemeID {
		t.Fatalf("theme_id = %q, want %q", loaded.Appearance.ThemeID, DefaultThemeID)
	}
	if loaded.Notifications.InApp || loaded.Notifications.OS {
		t.Fatalf("notifications = %+v, want all off", loaded.Notifications)
	}
	if loaded.Notifications.Ntfy.ServerURL != DefaultNtfyServerURL {
		t.Fatalf("ntfy server_url = %q, want %q", loaded.Notifications.Ntfy.ServerURL, DefaultNtfyServerURL)
	}
}

func TestNormalizeAppearanceAcceptsKnownThemes(t *testing.T) {
	for _, themeID := range []string{SpacetimeThemeID, VercelThemeID, HUDThemeID, SlateNoirThemeID, ZerobyteThemeID} {
		t.Run(themeID, func(t *testing.T) {
			appearance, err := NormalizeAppearance(Appearance{ThemeID: themeID})
			if err != nil {
				t.Fatal(err)
			}
			if appearance.ThemeID != themeID {
				t.Fatalf("theme_id = %q, want %q", appearance.ThemeID, themeID)
			}
		})
	}
}

func TestNormalizeAppearanceRejectsUnknownTheme(t *testing.T) {
	_, err := NormalizeAppearance(Appearance{ThemeID: "unknown"})
	if !errors.Is(err, ErrInvalidThemeID) {
		t.Fatalf("err = %v, want ErrInvalidThemeID", err)
	}
}

func TestLoadDowngradesUnknownThemeToDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"version":1,"appearance":{"theme_id":"future"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Appearance.ThemeID != DefaultThemeID {
		t.Fatalf("theme_id = %q, want %q", state.Appearance.ThemeID, DefaultThemeID)
	}
}

func TestManagerUpdateAppearanceRecoversCorruptState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(`{ nope`), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := NewManager(dir).UpdateAppearance(Appearance{ThemeID: SpacetimeThemeID})
	if err != nil {
		t.Fatal(err)
	}
	if state.Appearance.ThemeID != SpacetimeThemeID {
		t.Fatalf("theme_id = %q, want %q", state.Appearance.ThemeID, SpacetimeThemeID)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Appearance.ThemeID != SpacetimeThemeID {
		t.Fatalf("loaded theme_id = %q, want %q", loaded.Appearance.ThemeID, SpacetimeThemeID)
	}
}

func TestManagerUpdateSavesNotifications(t *testing.T) {
	dir := t.TempDir()
	inApp := true
	os := true
	enabled := true
	topic := "jump-a8f3k2m9"
	token := "tk_secret"
	notifications := NotificationsPatch{
		InApp: &inApp,
		OS:    &os,
		Ntfy: &NtfyPatch{
			Enabled: &enabled,
			TopicID: &topic,
			Token:   &token,
		},
	}

	state, err := NewManager(dir).Update(nil, &notifications)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Notifications.InApp || !state.Notifications.OS || !state.Notifications.Ntfy.Enabled {
		t.Fatalf("notifications = %+v, want channels on", state.Notifications)
	}
	if state.Notifications.Ntfy.TopicID != topic || state.Notifications.Ntfy.Token != token {
		t.Fatalf("ntfy = %+v, want topic/token saved", state.Notifications.Ntfy)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Notifications.InApp || !loaded.Notifications.OS || !loaded.Notifications.Ntfy.Enabled {
		t.Fatalf("loaded notifications = %+v, want channels on", loaded.Notifications)
	}
	if public := loaded.Notifications.Public(); !public.Ntfy.TokenConfigured {
		t.Fatalf("public ntfy = %+v, want token_configured", public.Ntfy)
	}
}

func TestNtfyRequiresTopicWhenEnabled(t *testing.T) {
	enabled := true
	patch := NotificationsPatch{Ntfy: &NtfyPatch{Enabled: &enabled}}
	_, err := patch.Apply(DefaultState().Notifications)
	if !errors.Is(err, ErrInvalidNtfy) {
		t.Fatalf("err = %v, want ErrInvalidNtfy", err)
	}
}

func TestNtfyTokenPatchKeepsExistingWhenOmitted(t *testing.T) {
	base := DefaultState().Notifications
	base.Ntfy.Token = "tk_existing"
	topic := "jump-topic"
	patch := NotificationsPatch{Ntfy: &NtfyPatch{TopicID: &topic}}
	updated, err := patch.Apply(base)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ntfy.Token != "tk_existing" {
		t.Fatalf("token = %q, want existing", updated.Ntfy.Token)
	}
}

func TestManagerUpdateAppearanceSavesAtomically(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(dir)
	appearance := Appearance{ThemeID: SpacetimeThemeID}
	state, err := manager.Update(&appearance, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Appearance.ThemeID != SpacetimeThemeID {
		t.Fatalf("theme_id = %q, want %q", state.Appearance.ThemeID, SpacetimeThemeID)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
}
