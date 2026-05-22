package webprefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	fileName         = "web-preferences.json"
	currentVersion   = 1
	DefaultThemeID   = "default"
	SpacetimeThemeID = "spacetime"
	VercelThemeID    = "vercel"
	HUDThemeID       = "hud"
	SlateNoirThemeID = "slate-noir"
)

var (
	ErrInvalidThemeID = errors.New("invalid theme_id")
	ErrInvalidState   = errors.New("invalid web preferences state")
	themeIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	knownThemeIDs     = map[string]struct{}{
		DefaultThemeID:   {},
		SpacetimeThemeID: {},
		VercelThemeID:    {},
		HUDThemeID:       {},
		SlateNoirThemeID: {},
	}
)

type Appearance struct {
	ThemeID string `json:"theme_id"`
}

type State struct {
	Version    int        `json:"version"`
	Appearance Appearance `json:"appearance"`
}

func DefaultState() *State {
	return &State{
		Version:    currentVersion,
		Appearance: Appearance{ThemeID: DefaultThemeID},
	}
}

func NormalizeAppearance(appearance Appearance) (Appearance, error) {
	if appearance.ThemeID == "" {
		appearance.ThemeID = DefaultThemeID
	}
	if !themeIDPattern.MatchString(appearance.ThemeID) {
		return Appearance{}, fmt.Errorf("%w: %q", ErrInvalidThemeID, appearance.ThemeID)
	}
	if _, ok := knownThemeIDs[appearance.ThemeID]; !ok {
		return Appearance{}, fmt.Errorf("%w: %q", ErrInvalidThemeID, appearance.ThemeID)
	}
	return appearance, nil
}

func Load(stateDir string) (*State, error) {
	path := filepath.Join(stateDir, fileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("webprefs: reading %s: %w", path, err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("webprefs: parsing %s: %w: %v", path, ErrInvalidState, err)
	}
	state.normalizeLoaded()
	return &state, nil
}

func (s *State) normalizeLoaded() {
	if s.Version == 0 {
		s.Version = currentVersion
	}
	if s.Appearance.ThemeID == "" || !themeIDPattern.MatchString(s.Appearance.ThemeID) {
		s.Appearance.ThemeID = DefaultThemeID
		return
	}
	if _, ok := knownThemeIDs[s.Appearance.ThemeID]; !ok {
		s.Appearance.ThemeID = DefaultThemeID
	}
}

func (s *State) Save(stateDir string) error {
	s.Version = currentVersion

	appearance, err := NormalizeAppearance(s.Appearance)
	if err != nil {
		return err
	}
	s.Appearance = appearance

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("webprefs: creating state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("webprefs: marshaling: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(stateDir, fileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("webprefs: writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("webprefs: renaming %s -> %s: %w", tmp, path, err)
	}
	return nil
}
