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

func TestManagerUpdateAppearanceSavesAtomically(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(dir)
	state, err := manager.UpdateAppearance(Appearance{ThemeID: SpacetimeThemeID})
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
