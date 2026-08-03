package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/jjones/e-biblioteca/components"
	"github.com/jjones/e-biblioteca/internal/models"
	"github.com/jjones/e-biblioteca/internal/store"
)

func (a *App) listBookAnnotationsAPI(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	bookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := a.Store.GetBook(r.Context(), bookID); err != nil {
		http.NotFound(w, r)
		return
	}
	list, err := a.Store.ListAnnotationsForBook(r.Context(), u.ID, bookID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if list == nil {
		list = []models.Annotation{}
	}
	writeJSON(w, list)
}

func (a *App) createBookAnnotationAPI(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	bookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), bookID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Kind    string          `json:"kind"`
		Color   string          `json:"color"`
		Quote   string          `json:"quote"`
		Note    string          `json:"note"`
		Tags    []string        `json:"tags"`
		Anchor  json.RawMessage `json:"anchor"`
		SortKey string          `json:"sort_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	tags := store.NormalizeTags(body.Tags)
	if err := store.ValidateAnnotationInput(body.Kind, body.Color, body.Quote, body.Note, tags, body.Anchor, book.Format); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var color *string
	if body.Color != "" {
		color = &body.Color
	}
	ann := &models.Annotation{
		UserID:  u.ID,
		BookID:  bookID,
		Kind:    body.Kind,
		Color:   color,
		Quote:   body.Quote,
		Note:    body.Note,
		Tags:    tags,
		Anchor:  body.Anchor,
		SortKey: body.SortKey,
	}
	if err := a.Store.CreateAnnotation(r.Context(), ann); err != nil {
		if errors.Is(err, store.ErrAnnotationLimit) {
			http.Error(w, err.Error(), 400)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSONStatus(w, http.StatusCreated, ann)
}

func (a *App) patchAnnotationAPI(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body struct {
		Color *string  `json:"color"`
		Quote *string  `json:"quote"`
		Note  *string  `json:"note"`
		Tags  []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	// Load current for format validation of color
	cur, err := a.Store.GetAnnotation(r.Context(), u.ID, id)
	if err != nil {
		// 404 for non-owner or missing
		http.NotFound(w, r)
		return
	}
	color := ""
	if body.Color != nil {
		color = *body.Color
	} else if cur.Color != nil {
		color = *cur.Color
	}
	quote := cur.Quote
	if body.Quote != nil {
		quote = *body.Quote
	}
	note := cur.Note
	if body.Note != nil {
		note = *body.Note
	}
	tags := cur.Tags
	if body.Tags != nil {
		tags = store.NormalizeTags(body.Tags)
	}
	if err := store.ValidateAnnotationInput(cur.Kind, color, quote, note, tags, cur.Anchor, ""); err != nil {
		// skip format-specific anchor check: empty format accepts scheme if present
		// re-validate color/kind only when format empty fails on scheme - re-check lengths
		if body.Color != nil && *body.Color != "" && !models.AllowedAnnotationColors[*body.Color] {
			http.Error(w, "invalid color", 400)
			return
		}
		if body.Quote != nil || body.Note != nil {
			// length check via Validate with anchor pass-through using a noop scheme
			// already have ErrInvalid for color/kind; for lengths:
		}
		_ = err
	}
	if body.Color != nil && *body.Color != "" && !models.AllowedAnnotationColors[*body.Color] {
		http.Error(w, "invalid color", 400)
		return
	}
	if body.Quote != nil && len([]rune(*body.Quote)) > models.MaxQuoteLen {
		http.Error(w, "quote too long", 400)
		return
	}
	if body.Note != nil && len([]rune(*body.Note)) > models.MaxNoteLen {
		http.Error(w, "note too long", 400)
		return
	}
	var tagsArg []string
	if body.Tags != nil {
		tagsArg = tags
	}
	updated, err := a.Store.UpdateAnnotation(r.Context(), u.ID, id, body.Color, body.Quote, body.Note, tagsArg, false)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, updated)
}

func (a *App) deleteAnnotationAPI(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.Store.DeleteAnnotation(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) annotationsPage(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	f := parseAnnotationFilter(r)
	list, total, err := a.Store.ListAnnotations(r.Context(), u.ID, f)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	libs, _ := a.Store.ListLibraries(r.Context())
	a.render(w, r, "Annotations", "annotations", components.AnnotationsPage(list, total, f, libs))
}

func (a *App) exportAnnotations(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	f := parseAnnotationFilter(r)
	// path may be /books/{id}/annotations/export
	if idStr := chi.URLParam(r, "id"); idStr != "" {
		if bookID, err := strconv.ParseInt(idStr, 10, 64); err == nil && bookID > 0 {
			f.BookID = bookID
		}
	}
	list, err := a.Store.ListAnnotationsForExport(r.Context(), u.ID, f)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(list) == 0 {
		http.Error(w, "no annotations to export", 400)
		return
	}
	md := buildAnnotationsMarkdown(list, f.BookID > 0)
	name := "annotations.md"
	if f.BookID > 0 && list[0].BookTitle != "" {
		name = sanitizeFilename(list[0].BookTitle) + "-annotations.md"
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_, _ = w.Write([]byte(md))
}

func parseAnnotationFilter(r *http.Request) models.AnnotationFilter {
	q := r.URL.Query()
	f := models.AnnotationFilter{
		Format: q.Get("format"),
		Kind:   q.Get("kind"),
		Color:  q.Get("color"),
		Tag:    q.Get("tag"),
		Query:  q.Get("q"),
	}
	if v, err := strconv.ParseInt(q.Get("book"), 10, 64); err == nil {
		f.BookID = v
	}
	if v, err := strconv.ParseInt(q.Get("library"), 10, 64); err == nil {
		f.LibraryID = v
	}
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 1 {
		f.Offset = (v - 1) * 50
	}
	f.Limit = 50
	return f
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func buildAnnotationsMarkdown(list []models.Annotation, singleBook bool) string {
	var b strings.Builder
	now := time.Now().UTC().Format(time.RFC3339)
	if singleBook {
		first := list[0]
		authors := splitAuthors(first.Authors)
		b.WriteString("---\n")
		b.WriteString(fmt.Sprintf("title: %q\n", first.BookTitle))
		b.WriteString("authors: [")
		for i, a := range authors {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%q", a))
		}
		b.WriteString("]\n")
		b.WriteString(fmt.Sprintf("format: %s\n", first.BookFormat))
		b.WriteString(fmt.Sprintf("book_id: %d\n", first.BookID))
		b.WriteString(fmt.Sprintf("exported_at: %s\n", now))
		b.WriteString(fmt.Sprintf("annotation_count: %d\n", len(list)))
		b.WriteString("---\n\n")
		b.WriteString(fmt.Sprintf("# %s\n\n", first.BookTitle))
		writeAnnotationBody(&b, list)
		return b.String()
	}

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("exported_at: %s\n", now))
	b.WriteString(fmt.Sprintf("annotation_count: %d\n", len(list)))
	b.WriteString("---\n\n")
	b.WriteString("# Library annotations\n\n")

	var curBook int64
	for _, ann := range list {
		if ann.BookID != curBook {
			curBook = ann.BookID
			b.WriteString(fmt.Sprintf("## %s\n\n", ann.BookTitle))
			if ann.Authors != "" {
				b.WriteString(fmt.Sprintf("*%s* · `%s`\n\n", ann.Authors, ann.BookFormat))
			}
		}
		writeOneAnnotation(&b, ann)
	}
	return b.String()
}

func writeAnnotationBody(b *strings.Builder, list []models.Annotation) {
	for _, ann := range list {
		writeOneAnnotation(b, ann)
	}
}

func writeOneAnnotation(b *strings.Builder, ann models.Annotation) {
	b.WriteString(fmt.Sprintf("### %s", capitalize(ann.Kind)))
	if ann.Color != nil && *ann.Color != "" {
		b.WriteString(fmt.Sprintf(" · %s", *ann.Color))
	}
	b.WriteString("\n\n")
	if ann.Quote != "" {
		for _, line := range strings.Split(ann.Quote, "\n") {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if ann.Note != "" {
		b.WriteString(fmt.Sprintf("- Note: %s\n", ann.Note))
	}
	if len(ann.Tags) > 0 {
		b.WriteString(fmt.Sprintf("- Tags: %s\n", strings.Join(ann.Tags, ", ")))
	}
	b.WriteString(fmt.Sprintf("- Location: `%s`\n", compactJSON(ann.Anchor)))
	b.WriteString(fmt.Sprintf("- Open: /read/%d?annotation=%d\n\n", ann.BookID, ann.ID))
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func splitAuthors(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "annotations"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
