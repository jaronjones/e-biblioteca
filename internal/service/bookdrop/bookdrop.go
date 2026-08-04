package bookdrop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jjones/e-biblioteca/internal/models"
	"github.com/jjones/e-biblioteca/internal/service/metadata"
	"github.com/jjones/e-biblioteca/internal/store"
)

type Service struct {
	Store       *store.Store
	BookdropDir string
	DataDir     string
	mu          sync.Mutex
	pending     map[string]*time.Timer
}

func New(st *store.Store, bookdropDir, dataDir string) *Service {
	return &Service{
		Store:       st,
		BookdropDir: bookdropDir,
		DataDir:     dataDir,
		pending:     map[string]*time.Timer{},
	}
}

func (s *Service) Start(ctx context.Context) error {
	if err := os.MkdirAll(s.BookdropDir, 0o755); err != nil {
		return err
	}
	// initial scan
	go s.scanExisting(ctx)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(s.BookdropDir); err != nil {
		watcher.Close()
		return err
	}
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Rename) {
					s.debounce(ctx, ev.Name)
				}
				if ev.Has(fsnotify.Remove) {
					_ = s.Store.UpsertBookdrop(ctx, ev.Name, filepath.Base(ev.Name), "", "rejected", nil, nil, nil)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("bookdrop watcher: %v", err)
			}
		}
	}()
	return nil
}

func (s *Service) debounce(ctx context.Context, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.pending[path]; ok {
		t.Stop()
	}
	s.pending[path] = time.AfterFunc(1500*time.Millisecond, func() {
		s.process(ctx, path)
		s.mu.Lock()
		delete(s.pending, path)
		s.mu.Unlock()
	})
}

func (s *Service) scanExisting(ctx context.Context) {
	entries, err := os.ReadDir(s.BookdropDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		s.process(ctx, filepath.Join(s.BookdropDir, e.Name()))
	}
}

func (s *Service) process(ctx context.Context, path string) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return
	}
	// ignore partials / hidden
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return
	}
	format, ok := metadata.FormatFromPath(path)
	if !ok {
		msg := "unsupported format"
		_ = s.Store.UpsertBookdrop(ctx, path, base, "", "error", nil, nil, &msg)
		return
	}
	meta, _ := metadata.Extract(path)
	metadata.Enrich(ctx, &meta)
	var coverRel *string
	if len(meta.CoverData) > 0 {
		dir := filepath.Join(s.DataDir, "bookdrop-covers")
		_ = os.MkdirAll(dir, 0o755)
		name := strings.TrimSuffix(base, filepath.Ext(base)) + meta.CoverExt
		if meta.CoverExt == "" {
			name += ".jpg"
		}
		cp := filepath.Join(dir, name)
		if err := os.WriteFile(cp, meta.CoverData, 0o644); err == nil {
			rel := filepath.ToSlash(filepath.Join("bookdrop-covers", name))
			coverRel = &rel
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"title":       meta.Title,
		"authors":     strings.Join(meta.Authors, ", "),
		"description": meta.Description,
		"isbn13":      meta.ISBN13,
		"isbn10":      meta.ISBN10,
		"publisher":   meta.Publisher,
		"categories":  meta.Categories,
	})
	_ = s.Store.UpsertBookdrop(ctx, path, base, format, "ready", payload, coverRel, nil)
}

func (s *Service) Import(ctx context.Context, dropID, libraryID int64, booksDir string, scn interface {
	SaveCoverBytes(bookID int64, data []byte, ext string) (string, error)
}) (int64, error) {
	f, err := s.Store.GetBookdrop(ctx, dropID)
	if err != nil {
		return 0, err
	}
	lib, err := s.Store.GetLibrary(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	if len(lib.Paths) == 0 {
		return 0, os.ErrInvalid
	}
	root := lib.Paths[0].Path
	if !filepath.IsAbs(root) {
		root = filepath.Join(booksDir, root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, err
	}
	// Avoid clobbering an existing library file of the same name.
	destName := uniqueFileName(root, f.FileName)
	dest := filepath.Join(root, destName)
	tmp := dest + ".partial"
	src, err := os.Open(f.Path)
	if err != nil {
		return 0, err
	}
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		src.Close()
		return 0, err
	}
	n, err := io.Copy(out, src)
	src.Close()
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	format := ""
	if f.Format != nil {
		format = *f.Format
	}
	meta, _ := metadata.Extract(dest)
	var m map[string]any
	_ = json.Unmarshal(f.MetadataJSON, &m)
	if t, ok := m["title"].(string); ok && t != "" {
		meta.Title = t
	}
	if a, ok := m["authors"].(string); ok && a != "" {
		meta.Authors = strings.Split(a, ", ")
	}
	hash, _ := metadata.HashFile(dest)
	lpID := lib.Paths[0].ID
	book := models.Book{
		LibraryID:     libraryID,
		LibraryPathID: &lpID,
		FileName:      destName,
		FileSubPath:   "",
		Format:        format,
		FileSize:      n,
	}
	if hash != "" {
		book.FileHash = &hash
	}
	id, err := s.Store.UpsertBook(ctx, book, meta)
	if err != nil {
		return 0, err
	}
	if f.CoverPath != nil && *f.CoverPath != "" {
		src := filepath.Join(s.DataDir, filepath.FromSlash(*f.CoverPath))
		if data, err := os.ReadFile(src); err == nil {
			if rel, err := scn.SaveCoverBytes(id, data, filepath.Ext(src)); err == nil {
				_ = s.Store.SetBookCover(ctx, id, rel)
			}
		}
	} else if len(meta.CoverData) > 0 {
		if rel, err := scn.SaveCoverBytes(id, meta.CoverData, meta.CoverExt); err == nil {
			_ = s.Store.SetBookCover(ctx, id, rel)
		}
	}
	_ = s.Store.UpdateBookdropStatus(ctx, dropID, "imported")
	_ = os.Remove(f.Path)
	return id, nil
}

func uniqueFileName(dir, name string) string {
	base := filepath.Base(name)
	candidate := base
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
}
