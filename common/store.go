// Package common contiene utilidades compartidas por los microservicios:
// persistencia en archivos JSON y helpers HTTP (respuestas, CORS, logs).
package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONStore mantiene en memoria una colección de registros y la persiste
// en un archivo JSON. Todas las operaciones están protegidas por un mutex,
// por lo que es seguro usarla desde varios handlers concurrentes.
type JSONStore[T any] struct {
	mu   sync.RWMutex
	path string
	data []T
}

// NewJSONStore lee el archivo indicado. Si no existe, lo crea vacío ([]).
func NewJSONStore[T any](path string) (*JSONStore[T], error) {
	s := &JSONStore[T]{path: path, data: []T{}}

	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", path, err)
	}
	return s, nil
}

// All devuelve una copia de todos los registros.
func (s *JSONStore[T]) All() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]T(nil), s.data...)
}

// Update recibe una copia de los registros y devuelve la nueva colección.
// Si fn devuelve error no se modifica nada; si no, se guarda en disco.
func (s *JSONStore[T]) Update(fn func(items []T) ([]T, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nuevos, err := fn(append([]T(nil), s.data...))
	if err != nil {
		return err
	}
	anteriores := s.data
	s.data = nuevos
	if err := s.save(); err != nil {
		s.data = anteriores
		return err
	}
	return nil
}

// save escribe primero en un archivo temporal y luego lo renombra, para no
// dejar el JSON a medio escribir si el proceso se interrumpe.
func (s *JSONStore[T]) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
