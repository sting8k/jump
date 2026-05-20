package webprefs

import (
	"errors"
	"sync"
)

type Manager struct {
	mu       sync.Mutex
	stateDir string
}

func NewManager(stateDir string) *Manager {
	return &Manager{stateDir: stateDir}
}

func (m *Manager) Load() (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Load(m.stateDir)
}

func (m *Manager) UpdateAppearance(appearance Appearance) (*State, error) {
	normalized, err := NormalizeAppearance(appearance)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := Load(m.stateDir)
	if err != nil {
		if !errors.Is(err, ErrInvalidState) {
			return nil, err
		}
		state = DefaultState()
	}
	state.Appearance = normalized
	if err := state.Save(m.stateDir); err != nil {
		return nil, err
	}
	return state, nil
}
