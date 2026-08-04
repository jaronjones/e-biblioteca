package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jjones/e-biblioteca/internal/models"
)

var (
	ErrAnnotationLimit  = errors.New("annotation limit exceeded")
	ErrInvalidAnnotation = errors.New("invalid annotation")
)

// ValidateAnnotationInput checks kind, color, lengths, tags, and anchor shape for the book format.
func ValidateAnnotationInput(kind, color, quote, note string, tags []string, anchor json.RawMessage, bookFormat string) error {
	switch kind {
	case models.AnnotationKindHighlight, models.AnnotationKindNote, models.AnnotationKindBookmark:
	default:
		return fmt.Errorf("%w: kind must be highlight, note, or bookmark", ErrInvalidAnnotation)
	}
	if color != "" && !models.AllowedAnnotationColors[color] {
		return fmt.Errorf("%w: invalid color", ErrInvalidAnnotation)
	}
	if kind == models.AnnotationKindHighlight && color == "" {
		return fmt.Errorf("%w: highlight requires a color", ErrInvalidAnnotation)
	}
	if utf8.RuneCountInString(quote) > models.MaxQuoteLen {
		return fmt.Errorf("%w: quote too long", ErrInvalidAnnotation)
	}
	if utf8.RuneCountInString(note) > models.MaxNoteLen {
		return fmt.Errorf("%w: note too long", ErrInvalidAnnotation)
	}
	if len(tags) > models.MaxTags {
		return fmt.Errorf("%w: too many tags", ErrInvalidAnnotation)
	}
	for _, t := range tags {
		if utf8.RuneCountInString(t) > models.MaxTagLen || t == "" {
			return fmt.Errorf("%w: invalid tag", ErrInvalidAnnotation)
		}
	}
	if len(anchor) == 0 {
		return fmt.Errorf("%w: anchor required", ErrInvalidAnnotation)
	}
	var a map[string]any
	if err := json.Unmarshal(anchor, &a); err != nil {
		return fmt.Errorf("%w: anchor must be JSON object", ErrInvalidAnnotation)
	}
	scheme, _ := a["scheme"].(string)
	switch normalizeFormat(bookFormat) {
	case "epub":
		if scheme != "epubcfi" {
			return fmt.Errorf("%w: epub requires scheme epubcfi", ErrInvalidAnnotation)
		}
		if cfi, _ := a["cfi"].(string); strings.TrimSpace(cfi) == "" {
			return fmt.Errorf("%w: epub requires cfi", ErrInvalidAnnotation)
		}
	case "pdf":
		if scheme != "pdf" {
			return fmt.Errorf("%w: pdf requires scheme pdf", ErrInvalidAnnotation)
		}
		if !hasPositiveNumber(a, "page") {
			return fmt.Errorf("%w: pdf requires page >= 1", ErrInvalidAnnotation)
		}
	case "cbz":
		if scheme != "cbz" {
			return fmt.Errorf("%w: cbz requires scheme cbz", ErrInvalidAnnotation)
		}
		if !hasNumber(a, "page") {
			return fmt.Errorf("%w: cbz requires page", ErrInvalidAnnotation)
		}
	case "audio":
		if scheme != "audio" {
			return fmt.Errorf("%w: audio requires scheme audio", ErrInvalidAnnotation)
		}
		if !hasNumber(a, "seconds") {
			return fmt.Errorf("%w: audio requires seconds", ErrInvalidAnnotation)
		}
	default:
		// Unknown format: accept any scheme with object shape.
		if scheme == "" {
			return fmt.Errorf("%w: anchor.scheme required", ErrInvalidAnnotation)
		}
	}
	return nil
}

func normalizeFormat(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	switch f {
	case "mp3", "m4b", "m4a", "opus":
		return "audio"
	default:
		return f
	}
}

func hasPositiveNumber(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch n := v.(type) {
	case float64:
		return n >= 1
	case int:
		return n >= 1
	case json.Number:
		f, err := n.Float64()
		return err == nil && f >= 1
	default:
		return false
	}
}

func hasNumber(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch v.(type) {
	case float64, int, json.Number:
		return true
	default:
		return false
	}
}

// NormalizeTags trims, casefolds for uniqueness, drops empties.
func NormalizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > models.MaxTagLen {
			t = string([]rune(t)[:models.MaxTagLen])
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) >= models.MaxTags {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func (s *Store) CountAnnotationsForBook(ctx context.Context, userID, bookID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations WHERE user_id=$1 AND book_id=$2
	`, userID, bookID).Scan(&n)
	return n, err
}

func (s *Store) CreateAnnotation(ctx context.Context, a *models.Annotation) error {
	n, err := s.CountAnnotationsForBook(ctx, a.UserID, a.BookID)
	if err != nil {
		return err
	}
	if n >= models.MaxAnnotationsPerBook {
		return ErrAnnotationLimit
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if len(a.Anchor) == 0 {
		a.Anchor = json.RawMessage(`{}`)
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO annotations (user_id, book_id, kind, color, quote, note, tags, anchor, sort_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at
	`, a.UserID, a.BookID, a.Kind, a.Color, a.Quote, a.Note, a.Tags, a.Anchor, a.SortKey).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (s *Store) UpdateAnnotation(ctx context.Context, userID, id int64, color *string, quote, note *string, tags []string, clearColor bool) (*models.Annotation, error) {
	cur, err := s.GetAnnotation(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if color != nil {
		if *color == "" {
			cur.Color = nil
		} else {
			cur.Color = color
		}
	} else if clearColor {
		cur.Color = nil
	}
	if quote != nil {
		cur.Quote = *quote
	}
	if note != nil {
		cur.Note = *note
	}
	if tags != nil {
		cur.Tags = tags
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE annotations SET color=$1, quote=$2, note=$3, tags=$4, updated_at=NOW()
		WHERE id=$5 AND user_id=$6
	`, cur.Color, cur.Quote, cur.Note, cur.Tags, id, userID)
	if err != nil {
		return nil, err
	}
	return s.GetAnnotation(ctx, userID, id)
}

func (s *Store) DeleteAnnotation(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM annotations WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAnnotation(ctx context.Context, userID, id int64) (*models.Annotation, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT a.id, a.user_id, a.book_id, a.kind, a.color, a.quote, a.note, a.tags, a.anchor, a.sort_key, a.created_at, a.updated_at
		FROM annotations a
		WHERE a.id=$1 AND a.user_id=$2
	`, id, userID)
	return scanAnnotation(row)
}

// GetAnnotationForUserOrNotFound returns 404-equivalent for non-owners (no existence leak).
func (s *Store) ListAnnotationsForBook(ctx context.Context, userID, bookID int64) ([]models.Annotation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.user_id, a.book_id, a.kind, a.color, a.quote, a.note, a.tags, a.anchor, a.sort_key, a.created_at, a.updated_at
		FROM annotations a
		WHERE a.user_id=$1 AND a.book_id=$2
		ORDER BY a.sort_key ASC NULLS LAST, a.created_at ASC
	`, userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnnotations(rows)
}

func (s *Store) ListAnnotations(ctx context.Context, userID int64, f models.AnnotationFilter) ([]models.Annotation, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	var conds []string
	var args []any
	args = append(args, userID)
	conds = append(conds, "a.user_id=$1")
	n := 2

	if f.BookID > 0 {
		conds = append(conds, fmt.Sprintf("a.book_id=$%d", n))
		args = append(args, f.BookID)
		n++
	}
	if f.LibraryID > 0 {
		conds = append(conds, fmt.Sprintf("b.library_id=$%d", n))
		args = append(args, f.LibraryID)
		n++
	}
	if f.Format != "" {
		conds = append(conds, fmt.Sprintf("b.format=$%d", n))
		args = append(args, f.Format)
		n++
	}
	if f.Kind != "" {
		conds = append(conds, fmt.Sprintf("a.kind=$%d", n))
		args = append(args, f.Kind)
		n++
	}
	if f.Color != "" {
		conds = append(conds, fmt.Sprintf("a.color=$%d", n))
		args = append(args, f.Color)
		n++
	}
	if f.Tag != "" {
		conds = append(conds, fmt.Sprintf("$%d = ANY(a.tags)", n))
		args = append(args, f.Tag)
		n++
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, fmt.Sprintf("(a.quote ILIKE $%d OR a.note ILIKE $%d OR EXISTS (SELECT 1 FROM unnest(a.tags) t WHERE t ILIKE $%d))", n, n, n))
		args = append(args, "%"+q+"%")
		n++
	}

	where := strings.Join(conds, " AND ")
	countQ := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM annotations a
		JOIN books b ON b.id = a.book_id AND b.deleted = FALSE
		WHERE %s
	`, where)
	var total int
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	listQ := fmt.Sprintf(`
		SELECT a.id, a.user_id, a.book_id, a.kind, a.color, a.quote, a.note, a.tags, a.anchor, a.sort_key, a.created_at, a.updated_at,
		       COALESCE(m.title, b.file_name) AS book_title, b.format,
		       COALESCE((SELECT string_agg(au.name, ', ' ORDER BY au.name) FROM book_authors ba JOIN authors au ON au.id=ba.author_id WHERE ba.book_id=b.id), '')
		FROM annotations a
		JOIN books b ON b.id = a.book_id AND b.deleted = FALSE
		LEFT JOIN book_metadata m ON m.book_id = b.id
		WHERE %s
		ORDER BY a.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, n, n+1)

	rows, err := s.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []models.Annotation
	for rows.Next() {
		var a models.Annotation
		var tags []string
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.BookID, &a.Kind, &a.Color, &a.Quote, &a.Note, &tags, &a.Anchor, &a.SortKey, &a.CreatedAt, &a.UpdatedAt,
			&a.BookTitle, &a.BookFormat, &a.Authors,
		); err != nil {
			return nil, 0, err
		}
		if tags == nil {
			tags = []string{}
		}
		a.Tags = tags
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func (s *Store) ListAnnotationsForExport(ctx context.Context, userID int64, f models.AnnotationFilter) ([]models.Annotation, error) {
	f.Limit = 10000
	f.Offset = 0
	// Reuse list but raise cap via dedicated query ordered by book then sort_key
	var conds []string
	var args []any
	args = append(args, userID)
	conds = append(conds, "a.user_id=$1")
	n := 2
	if f.BookID > 0 {
		conds = append(conds, fmt.Sprintf("a.book_id=$%d", n))
		args = append(args, f.BookID)
		n++
	}
	if f.LibraryID > 0 {
		conds = append(conds, fmt.Sprintf("b.library_id=$%d", n))
		args = append(args, f.LibraryID)
		n++
	}
	if f.Format != "" {
		conds = append(conds, fmt.Sprintf("b.format=$%d", n))
		args = append(args, f.Format)
		n++
	}
	if f.Kind != "" {
		conds = append(conds, fmt.Sprintf("a.kind=$%d", n))
		args = append(args, f.Kind)
		n++
	}
	if f.Color != "" {
		conds = append(conds, fmt.Sprintf("a.color=$%d", n))
		args = append(args, f.Color)
		n++
	}
	if f.Tag != "" {
		conds = append(conds, fmt.Sprintf("$%d = ANY(a.tags)", n))
		args = append(args, f.Tag)
		n++
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, fmt.Sprintf("(a.quote ILIKE $%d OR a.note ILIKE $%d OR EXISTS (SELECT 1 FROM unnest(a.tags) t WHERE t ILIKE $%d))", n, n, n))
		args = append(args, "%"+q+"%")
	}
	where := strings.Join(conds, " AND ")
	q := fmt.Sprintf(`
		SELECT a.id, a.user_id, a.book_id, a.kind, a.color, a.quote, a.note, a.tags, a.anchor, a.sort_key, a.created_at, a.updated_at,
		       COALESCE(m.title, b.file_name) AS book_title, b.format,
		       COALESCE((SELECT string_agg(au.name, ', ' ORDER BY au.name) FROM book_authors ba JOIN authors au ON au.id=ba.author_id WHERE ba.book_id=b.id), '')
		FROM annotations a
		JOIN books b ON b.id = a.book_id AND b.deleted = FALSE
		LEFT JOIN book_metadata m ON m.book_id = b.id
		WHERE %s
		ORDER BY book_title ASC, a.sort_key ASC, a.created_at ASC
		LIMIT 10000
	`, where)
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Annotation
	for rows.Next() {
		var a models.Annotation
		var tags []string
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.BookID, &a.Kind, &a.Color, &a.Quote, &a.Note, &tags, &a.Anchor, &a.SortKey, &a.CreatedAt, &a.UpdatedAt,
			&a.BookTitle, &a.BookFormat, &a.Authors,
		); err != nil {
			return nil, err
		}
		if tags == nil {
			tags = []string{}
		}
		a.Tags = tags
		out = append(out, a)
	}
	return out, rows.Err()
}

type annotationScanner interface {
	Scan(dest ...any) error
}

func scanAnnotation(row annotationScanner) (*models.Annotation, error) {
	var a models.Annotation
	var tags []string
	err := row.Scan(&a.ID, &a.UserID, &a.BookID, &a.Kind, &a.Color, &a.Quote, &a.Note, &tags, &a.Anchor, &a.SortKey, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []string{}
	}
	a.Tags = tags
	return &a, nil
}

func scanAnnotations(rows pgx.Rows) ([]models.Annotation, error) {
	var out []models.Annotation
	for rows.Next() {
		var a models.Annotation
		var tags []string
		if err := rows.Scan(&a.ID, &a.UserID, &a.BookID, &a.Kind, &a.Color, &a.Quote, &a.Note, &tags, &a.Anchor, &a.SortKey, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if tags == nil {
			tags = []string{}
		}
		a.Tags = tags
		out = append(out, a)
	}
	return out, rows.Err()
}

// Ensure compile-time use of time package if needed later.
var _ = time.Time{}
