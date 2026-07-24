package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jjones/e-biblioteca/internal/models"
	"github.com/jjones/e-biblioteca/internal/service/metadata"
	"github.com/jjones/e-biblioteca/internal/store"
)

type Scanner struct {
	Store   *store.Store
	DataDir string
	BooksDir string
}

func (s *Scanner) ScanLibrary(ctx context.Context, libraryID int64) (int, error) {
	lib, err := s.Store.GetLibrary(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, lp := range lib.Paths {
		root := lp.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(s.BooksDir, root)
		}
		// ensure under books dir when relative
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			log.Printf("scan: skip missing path %s: %v", root, err)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			format, ok := metadata.FormatFromPath(path)
			if !ok {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = filepath.Base(path)
			}
			sub := filepath.Dir(rel)
			if sub == "." {
				sub = ""
			}
			meta, _ := metadata.Extract(path)
			hash, _ := metadata.HashFile(path)
			book := models.Book{
				LibraryID:     libraryID,
				LibraryPathID: &lp.ID,
				FileName:      filepath.Base(path),
				FileSubPath:   filepath.ToSlash(sub),
				Format:        format,
				FileSize:      metadata.FileSize(path),
			}
			if hash != "" {
				book.FileHash = &hash
			}
			// save cover
			if len(meta.CoverData) > 0 {
				coverRel, err := s.saveCoverTemp(0, meta.CoverData, meta.CoverExt)
				if err == nil {
					book.Metadata.CoverPath = &coverRel
				}
			}
			id, err := s.Store.UpsertBook(ctx, book, meta)
			if err != nil {
				log.Printf("scan upsert %s: %v", path, err)
				return nil
			}
			if len(meta.CoverData) > 0 {
				coverRel, err := s.saveCover(id, meta.CoverData, meta.CoverExt)
				if err == nil {
					_ = s.Store.SetBookCover(ctx, id, coverRel)
				}
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func (s *Scanner) AbsoluteBookPath(book *models.Book) (string, error) {
	lib, err := s.Store.GetLibrary(context.Background(), book.LibraryID)
	if err != nil {
		return "", err
	}
	// find matching path entry
	var root string
	if book.LibraryPathID != nil {
		for _, lp := range lib.Paths {
			if lp.ID == *book.LibraryPathID {
				root = lp.Path
				break
			}
		}
	}
	if root == "" && len(lib.Paths) > 0 {
		root = lib.Paths[0].Path
	}
	if root == "" {
		return "", fmt.Errorf("no library path")
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(s.BooksDir, root)
	}
	p := filepath.Join(root, filepath.FromSlash(book.FileSubPath), book.FileName)
	return p, nil
}

func (s *Scanner) saveCover(bookID int64, data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".jpg"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	dir := filepath.Join(s.DataDir, "covers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d%s", bookID, ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("covers", name)), nil
}

func (s *Scanner) saveCoverTemp(bookID int64, data []byte, ext string) (string, error) {
	return s.saveCover(bookID, data, ext)
}

func (s *Scanner) SaveCoverBytes(bookID int64, data []byte, ext string) (string, error) {
	return s.saveCover(bookID, data, ext)
}

func (s *Scanner) IsUnderBooks(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root, err := filepath.Abs(s.BooksDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
