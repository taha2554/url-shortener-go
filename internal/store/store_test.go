package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.json")
	s, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Link{Code: "abc", URL: "https://go.dev"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordClick("abc", time.Now()); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	link, err := reopened.Get("abc")
	if err != nil || link.Clicks != 1 {
		t.Fatalf("got %+v, %v", link, err)
	}
}

func TestErrors(t *testing.T) {
	s := NewMemory()
	s.Create(Link{Code: "x"})
	if err := s.Create(Link{Code: "x"}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := s.Get("y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := s.Delete("x"); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestConcurrentClicks(t *testing.T) {
	s := NewMemory()
	s.Create(Link{Code: "c"})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RecordClick("c", time.Now())
		}()
	}
	wg.Wait()
	if l, _ := s.Get("c"); l.Clicks != 100 {
		t.Fatalf("clicks = %d", l.Clicks)
	}
}
