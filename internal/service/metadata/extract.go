package metadata

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/jjones/e-biblioteca/internal/models"
)

// MaxZipEntryBytes caps individual zip entry reads (covers, ComicInfo, pages).
const MaxZipEntryBytes = 32 << 20 // 32 MiB

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = MaxZipEntryBytes
	}
	return io.ReadAll(io.LimitReader(r, limit))
}

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

type containerRootfile struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

type containerXML struct {
	Rootfiles []containerRootfile `xml:"rootfiles>rootfile"`
}

func extractEPUB(path string) (models.ExtractedMetadata, error) {
	var meta models.ExtractedMetadata
	r, err := zip.OpenReader(path)
	if err != nil {
		return meta, err
	}
	defer r.Close()

	var opfPath string
	// Prefer META-INF/container.xml (EPUB spec).
	for _, f := range r.File {
		if strings.EqualFold(f.Name, "META-INF/container.xml") {
			rc, err := f.Open()
			if err != nil {
				break
			}
			b, err := readLimited(rc, 1<<20)
			rc.Close()
			if err != nil {
				break
			}
			var c containerXML
			if xml.Unmarshal(b, &c) == nil && len(c.Rootfiles) > 0 && c.Rootfiles[0].FullPath != "" {
				opfPath = c.Rootfiles[0].FullPath
			} else if i := bytes.Index(b, []byte(`full-path="`)); i >= 0 {
				rest := b[i+11:]
				if j := bytes.IndexByte(rest, '"'); j >= 0 {
					opfPath = string(rest[:j])
				}
			}
			break
		}
	}
	// Fallback: first .opf in the archive.
	if opfPath == "" {
		for _, f := range r.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
				opfPath = f.Name
				break
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
		digits := isbnChars(v)
		switch {
		case len(digits) == 13 && isAllDigits(digits):
			meta.ISBN13 = digits
		case len(digits) == 10:
			meta.ISBN10 = digits
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
					data, _ := readLimited(rc, MaxZipEntryBytes)
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
			b, _ := readLimited(rc, 1<<20)
			rc.Close()
			var ci struct {
				Title   string `xml:"Title"`
				Series  string `xml:"Series"`
				Number  string `xml:"Number"`
				Writer  string `xml:"Writer"`
				Genre   string `xml:"Genre"`
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
			data, _ := readLimited(rc, MaxZipEntryBytes)
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

// isbnChars keeps digits and a trailing X check character for ISBN-10.
func isbnChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || r == 'X' || r == 'x' {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

// onlyDigits is used by provider normalizers; keeps digits and ISBN-10 X.
func onlyDigits(s string) string {
	return isbnChars(s)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
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
	sort.Slice(pages, func(i, j int) bool {
		return naturalLess(pages[i], pages[j])
	})
	return pages, nil
}

// naturalLess sorts so page9 < page10.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ca, cb := a[ai], b[bi]
		if isDigitByte(ca) && isDigitByte(cb) {
			// skip leading zeros
			for ai < len(a) && a[ai] == '0' {
				ai++
			}
			for bi < len(b) && b[bi] == '0' {
				bi++
			}
			as, bs := ai, bi
			for ai < len(a) && isDigitByte(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigitByte(b[bi]) {
				bi++
			}
			adigits, bdigits := a[as:ai], b[bs:bi]
			if len(adigits) != len(bdigits) {
				return len(adigits) < len(bdigits)
			}
			if adigits != bdigits {
				return adigits < bdigits
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(a) < len(b)
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

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
			return readLimited(rc, MaxZipEntryBytes)
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

// HashFile returns a full-content SHA-256 hex digest for deduplication.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
