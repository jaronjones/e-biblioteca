package metadata

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jjones/e-biblioteca/internal/models"
)

var SupportedExt = map[string]string{
	".epub": "epub",
	".pdf":  "pdf",
	".cbz":  "cbz",
	".m4b":  "m4b",
	".m4a":  "m4a",
	".mp3":  "mp3",
	".opus": "opus",
}

func FormatFromPath(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	f, ok := SupportedExt[ext]
	return f, ok
}

func Extract(path string) (models.ExtractedMetadata, error) {
	format, ok := FormatFromPath(path)
	if !ok {
		return models.ExtractedMetadata{}, nil
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	meta := models.ExtractedMetadata{Title: humanizeFilename(base)}

	switch format {
	case "epub":
		if m, err := extractEPUB(path); err == nil {
			meta = merge(meta, m)
		}
	case "cbz":
		if m, err := extractCBZ(path); err == nil {
			meta = merge(meta, m)
		}
	case "pdf":
		// lightweight: filename only; covers come from providers
	case "m4b", "m4a", "mp3", "opus":
		// filename defaults; optional tag parse later
	}
	return meta, nil
}

func humanizeFilename(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.TrimSpace(s)
}

func merge(base, over models.ExtractedMetadata) models.ExtractedMetadata {
	if over.Title != "" {
		base.Title = over.Title
	}
	if over.Subtitle != "" {
		base.Subtitle = over.Subtitle
	}
	if over.Description != "" {
		base.Description = over.Description
	}
	if over.Publisher != "" {
		base.Publisher = over.Publisher
	}
	if over.Language != "" {
		base.Language = over.Language
	}
	if over.ISBN10 != "" {
		base.ISBN10 = over.ISBN10
	}
	if over.ISBN13 != "" {
		base.ISBN13 = over.ISBN13
	}
	if over.SeriesName != "" {
		base.SeriesName = over.SeriesName
	}
	if over.SeriesNumber > 0 {
		base.SeriesNumber = over.SeriesNumber
	}
	if len(over.Authors) > 0 {
		base.Authors = over.Authors
	}
	if len(over.Categories) > 0 {
		base.Categories = over.Categories
	}
	if len(over.CoverData) > 0 {
		base.CoverData = over.CoverData
		base.CoverExt = over.CoverExt
	}
	return base
}

type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
			Type string `xml:"media-type,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
}

type opfMetadata struct {
	Titles      []string `xml:"title"`
	Creators    []string `xml:"creator"`
	Description []string `xml:"description"`
	Publisher   []string `xml:"publisher"`
	Language    []string `xml:"language"`
	Subjects    []string `xml:"subject"`
	Identifiers []struct {
		Scheme string `xml:"scheme,attr"`
		Value  string `xml:",chardata"`
	} `xml:"identifier"`
}

func extractEPUB(path string) (models.ExtractedMetadata, error) {
	var meta models.ExtractedMetadata
	r, err := zip.OpenReader(path)
	if err != nil {
		return meta, err
	}
	defer r.Close()

	var opfPath string
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
			opfPath = f.Name
			break
		}
		if strings.EqualFold(f.Name, "META-INF/container.xml") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			// crude parse for full-path=
			if i := bytes.Index(b, []byte(`full-path="`)); i >= 0 {
				rest := b[i+11:]
				if j := bytes.IndexByte(rest, '"'); j >= 0 {
					opfPath = string(rest[:j])
				}
			}
		}
	}
	if opfPath == "" {
		return meta, nil
	}

	var opfFile *zip.File
	for _, f := range r.File {
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}
	if opfFile == nil {
		return meta, nil
	}
	rc, err := opfFile.Open()
	if err != nil {
		return meta, err
	}
	defer rc.Close()
	var pkg opfPackage
	if err := xml.NewDecoder(rc).Decode(&pkg); err != nil {
		return meta, err
	}
	if len(pkg.Metadata.Titles) > 0 {
		meta.Title = strings.TrimSpace(pkg.Metadata.Titles[0])
	}
	meta.Authors = cleanList(pkg.Metadata.Creators)
	if len(pkg.Metadata.Description) > 0 {
		meta.Description = strings.TrimSpace(pkg.Metadata.Description[0])
	}
	if len(pkg.Metadata.Publisher) > 0 {
		meta.Publisher = strings.TrimSpace(pkg.Metadata.Publisher[0])
	}
	if len(pkg.Metadata.Language) > 0 {
		meta.Language = strings.TrimSpace(pkg.Metadata.Language[0])
	}
	meta.Categories = cleanList(pkg.Metadata.Subjects)
	for _, id := range pkg.Metadata.Identifiers {
		v := strings.TrimSpace(id.Value)
		v = strings.ReplaceAll(v, "-", "")
		if strings.Contains(strings.ToLower(id.Scheme), "isbn") || strings.HasPrefix(strings.ToLower(v), "isbn") {
			digits := onlyDigits(v)
			if len(digits) == 13 {
				meta.ISBN13 = digits
			} else if len(digits) == 10 {
				meta.ISBN10 = digits
			}
		} else {
			digits := onlyDigits(v)
			if len(digits) == 13 {
				meta.ISBN13 = digits
			} else if len(digits) == 10 {
				meta.ISBN10 = digits
			}
		}
	}

	// cover
	baseDir := filepath.ToSlash(filepath.Dir(opfPath))
	for _, item := range pkg.Manifest.Items {
		if strings.Contains(strings.ToLower(item.ID), "cover") || strings.HasPrefix(item.Type, "image/") {
			href := item.Href
			if baseDir != "." && baseDir != "" {
				href = baseDir + "/" + href
			}
			href = filepath.ToSlash(pathClean(href))
			for _, f := range r.File {
				if filepath.ToSlash(f.Name) == href || strings.HasSuffix(filepath.ToSlash(f.Name), href) {
					rc, err := f.Open()
					if err != nil {
						break
					}
					data, _ := io.ReadAll(rc)
					rc.Close()
					if len(data) > 0 {
						meta.CoverData = data
						meta.CoverExt = extFromType(item.Type, f.Name)
						return meta, nil
					}
				}
			}
		}
	}
	return meta, nil
}

func pathClean(p string) string {
	parts := strings.Split(p, "/")
	var out []string
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func extFromType(mediaType, name string) string {
	switch mediaType {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return ".jpg"
	}
	return ext
}

func extractCBZ(path string) (models.ExtractedMetadata, error) {
	var meta models.ExtractedMetadata
	r, err := zip.OpenReader(path)
	if err != nil {
		return meta, err
	}
	defer r.Close()

	var firstImage *zip.File
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, "comicinfo.xml") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			var ci struct {
				Title  string `xml:"Title"`
				Series string `xml:"Series"`
				Number string `xml:"Number"`
				Writer string `xml:"Writer"`
				Genre  string `xml:"Genre"`
				Summary string `xml:"Summary"`
			}
			if xml.Unmarshal(b, &ci) == nil {
				if ci.Title != "" {
					meta.Title = ci.Title
				}
				if ci.Series != "" {
					meta.SeriesName = ci.Series
				}
				if ci.Writer != "" {
					meta.Authors = splitAuthors(ci.Writer)
				}
				if ci.Genre != "" {
					meta.Categories = splitAuthors(ci.Genre)
				}
				if ci.Summary != "" {
					meta.Description = ci.Summary
				}
			}
		}
		if firstImage == nil {
			ext := filepath.Ext(name)
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
				// skip mac junk
				if !strings.Contains(name, "__macosx") {
					firstImage = f
				}
			}
		}
	}
	if firstImage != nil && len(meta.CoverData) == 0 {
		rc, err := firstImage.Open()
		if err == nil {
			data, _ := io.ReadAll(rc)
			rc.Close()
			meta.CoverData = data
			meta.CoverExt = filepath.Ext(firstImage.Name)
			if meta.CoverExt == "" {
				meta.CoverExt = ".jpg"
			}
		}
	}
	return meta, nil
}

func cleanList(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitAuthors(s string) []string {
	s = strings.ReplaceAll(s, ";", ",")
	parts := strings.Split(s, ",")
	return cleanList(parts)
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || r == 'X' || r == 'x' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ListCBZPages(path string) ([]string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var pages []string
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "__macosx") {
			continue
		}
		ext := filepath.Ext(name)
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif" {
			pages = append(pages, f.Name)
		}
	}
	return pages, nil
}

func ReadZipEntry(path, entry string) ([]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name == entry {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, os.ErrNotExist
}

func FileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, 8<<20)); err != nil {
		return "", err
	}
	fi, err := f.Stat()
	if err == nil {
		fmt.Fprintf(h, ":%d:%s", fi.Size(), filepath.Base(path))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
