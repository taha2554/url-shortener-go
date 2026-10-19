// Package store keeps short links. There is an in-memory implementation and a
// file-backed one that snapshots the map to a JSON file.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("link not found")
	ErrExists   = errors.New("code already in use")
)

type Link struct {
	Code      string     `json:"code"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Clicks    int64      `json:"clicks"`
	LastClick *time.Time `json:"last_click,omitempty"`
}

func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && now.After(*l.ExpiresAt)
}

type Store interface {
	Create(link Link) error
	Get(code string) (Link, error)
	RecordClick(code string, at time.Time) error
	Delete(code string) error
}

// Memory is safe for concurrent use.
type Memory struct {
	mu    sync.RWMutex
	links map[string]Link
	// onChange is called with the lock held after every write (used by File).
	onChange func(map[string]Link) error
}

func NewMemory() *Memory {
	return &Memory{links: make(map[string]Link)}
}

func (m *Memory) Create(link Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.links[link.Code]; ok {
		return ErrExists
	}
	m.links[link.Code] = link
	return m.changed()
}

func (m *Memory) Get(code string) (Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func (m *Memory) RecordClick(code string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[code]
	if !ok {
		return ErrNotFound
	}
	link.Clicks++
	link.LastClick = &at
	m.links[code] = link
	return m.changed()
}

func (m *Memory) Delete(code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.links[code]; !ok {
		return ErrNotFound
	}
	delete(m.links, code)
	return m.changed()
}

func (m *Memory) changed() error {
	if m.onChange == nil {
		return nil
	}
	return m.onChange(m.links)
}

// NewFile loads links from path (if it exists) and writes the whole map back
// after each change. Fine for a small personal shortener; a real deployment
// would use a database.
func NewFile(path string) (*Memory, error) {
	m := NewMemory()
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &m.links); err != nil {
			return nil, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}

	m.onChange = func(links map[string]Link) error {
		data, err := json.MarshalIndent(links, "", "  ")
		if err != nil {
			return err
		}
		// write to a temp file and rename so a crash can't leave half a file behind
		tmp, err := os.CreateTemp(filepath.Dir(path), ".links-*.json")
		if err != nil {
			return err
		}
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		return os.Rename(tmp.Name(), path)
	}
	return m, nil
}
