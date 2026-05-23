package webprefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	fileName             = "web-preferences.json"
	currentVersion       = 1
	DefaultThemeID       = "default"
	DefaultNtfyServerURL = "https://ntfy.sh"
	SpacetimeThemeID     = "spacetime"
	VercelThemeID        = "vercel"
	HUDThemeID           = "hud"
	SlateNoirThemeID     = "slate-noir"
	ZerobyteThemeID      = "zerobyte"
)

var (
	ErrInvalidThemeID = errors.New("invalid theme_id")
	ErrInvalidNtfy    = errors.New("invalid ntfy settings")
	ErrInvalidState   = errors.New("invalid web preferences state")
	themeIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	ntfyTopicPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	knownThemeIDs     = map[string]struct{}{
		DefaultThemeID:   {},
		SpacetimeThemeID: {},
		VercelThemeID:    {},
		HUDThemeID:       {},
		SlateNoirThemeID: {},
		ZerobyteThemeID:  {},
	}
)

type Appearance struct {
	ThemeID string `json:"theme_id"`
}
type Ntfy struct {
	Enabled     bool   `json:"enabled"`
	ServerURL   string `json:"server_url"`
	TopicID     string `json:"topic_id"`
	Token       string `json:"token,omitempty"`
	SendDetails bool   `json:"send_details"`
}

type Notifications struct {
	InApp bool `json:"in_app"`
	OS    bool `json:"os"`
	Ntfy  Ntfy `json:"ntfy"`
}

type PublicNtfy struct {
	Enabled         bool   `json:"enabled"`
	ServerURL       string `json:"server_url"`
	TopicID         string `json:"topic_id"`
	TokenConfigured bool   `json:"token_configured"`
	SendDetails     bool   `json:"send_details"`
}

type PublicNotifications struct {
	InApp bool       `json:"in_app"`
	OS    bool       `json:"os"`
	Ntfy  PublicNtfy `json:"ntfy"`
}

type NtfyPatch struct {
	Enabled     *bool   `json:"enabled"`
	ServerURL   *string `json:"server_url"`
	TopicID     *string `json:"topic_id"`
	Token       *string `json:"token"`
	ClearToken  bool    `json:"clear_token"`
	SendDetails *bool   `json:"send_details"`
}

type NotificationsPatch struct {
	InApp *bool      `json:"in_app"`
	OS    *bool      `json:"os"`
	Ntfy  *NtfyPatch `json:"ntfy"`
}

type State struct {
	Version       int           `json:"version"`
	Appearance    Appearance    `json:"appearance"`
	Notifications Notifications `json:"notifications"`
}

func DefaultState() *State {
	return &State{
		Version:       currentVersion,
		Appearance:    Appearance{ThemeID: DefaultThemeID},
		Notifications: Notifications{InApp: false, OS: false, Ntfy: Ntfy{ServerURL: DefaultNtfyServerURL}},
	}
}

func (n Notifications) Public() PublicNotifications {
	normalized, err := NormalizeNotifications(n)
	if err != nil {
		normalized = DefaultState().Notifications
	}
	return PublicNotifications{
		InApp: normalized.InApp,
		OS:    normalized.OS,
		Ntfy: PublicNtfy{
			Enabled:         normalized.Ntfy.Enabled,
			ServerURL:       normalized.Ntfy.ServerURL,
			TopicID:         normalized.Ntfy.TopicID,
			TokenConfigured: normalized.Ntfy.Token != "",
			SendDetails:     normalized.Ntfy.SendDetails,
		},
	}
}

func (p NotificationsPatch) Apply(base Notifications) (Notifications, error) {
	next := base
	if p.InApp != nil {
		next.InApp = *p.InApp
	}
	if p.OS != nil {
		next.OS = *p.OS
	}
	if p.Ntfy != nil {
		if p.Ntfy.Enabled != nil {
			next.Ntfy.Enabled = *p.Ntfy.Enabled
		}
		if p.Ntfy.ServerURL != nil {
			next.Ntfy.ServerURL = *p.Ntfy.ServerURL
		}
		if p.Ntfy.TopicID != nil {
			next.Ntfy.TopicID = *p.Ntfy.TopicID
		}
		if p.Ntfy.Token != nil {
			next.Ntfy.Token = *p.Ntfy.Token
		}
		if p.Ntfy.ClearToken {
			next.Ntfy.Token = ""
		}
		if p.Ntfy.SendDetails != nil {
			next.Ntfy.SendDetails = *p.Ntfy.SendDetails
		}
	}
	return NormalizeNotifications(next)
}

func NormalizeNotifications(notifications Notifications) (Notifications, error) {
	ntfy, err := NormalizeNtfy(notifications.Ntfy)
	if err != nil {
		return Notifications{}, err
	}
	notifications.Ntfy = ntfy
	return notifications, nil
}

func NormalizeNtfy(ntfy Ntfy) (Ntfy, error) {
	ntfy.ServerURL = strings.TrimSpace(ntfy.ServerURL)
	if ntfy.ServerURL == "" {
		ntfy.ServerURL = DefaultNtfyServerURL
	}
	parsed, err := url.Parse(ntfy.ServerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Ntfy{}, fmt.Errorf("%w: server_url", ErrInvalidNtfy)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	ntfy.ServerURL = strings.TrimRight(parsed.String(), "/")

	ntfy.TopicID = strings.Trim(strings.TrimSpace(ntfy.TopicID), "/")
	if ntfy.TopicID != "" && !ntfyTopicPattern.MatchString(ntfy.TopicID) {
		return Ntfy{}, fmt.Errorf("%w: topic_id", ErrInvalidNtfy)
	}
	if ntfy.Enabled && ntfy.TopicID == "" {
		return Ntfy{}, fmt.Errorf("%w: topic_id required", ErrInvalidNtfy)
	}
	ntfy.Token = strings.TrimSpace(ntfy.Token)
	return ntfy, nil
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
	}
	if _, ok := knownThemeIDs[s.Appearance.ThemeID]; !ok {
		s.Appearance.ThemeID = DefaultThemeID
	}
	if notifications, err := NormalizeNotifications(s.Notifications); err == nil {
		s.Notifications = notifications
	} else {
		s.Notifications = DefaultState().Notifications
	}
}

func (s *State) Save(stateDir string) error {
	s.Version = currentVersion

	appearance, err := NormalizeAppearance(s.Appearance)
	if err != nil {
		return err
	}
	s.Appearance = appearance
	notifications, err := NormalizeNotifications(s.Notifications)
	if err != nil {
		return err
	}
	s.Notifications = notifications

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
