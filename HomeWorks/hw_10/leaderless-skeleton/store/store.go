package store

import (
	"errors"
	"sync"
)

var ErrKeyNotFound = errors.New("key not found")
var ErrEmptyKey = errors.New("key cannot be empty")

type Entry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type Store struct {
	mu   sync.RWMutex
	data map[string]Entry
}

func NewStore() *Store {
	return &Store{
		data: make(map[string]Entry),
	}
}

func (s *Store) SetLocal(key string, entry Entry) error {
	if key == "" {
		return ErrEmptyKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry
	return nil
}

func (s *Store) GetLocal(key string) (Entry, error) {
	if key == "" {
		return Entry{}, ErrEmptyKey
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.data[key]
	if !ok {
		return Entry{}, ErrKeyNotFound
	}
	return entry, nil
}

func (s *Store) NextVersion(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry, ok := s.data[key]; ok {
		return entry.Version + 1
	}
	return 1
}