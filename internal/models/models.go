package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	Email        *string
	IsAdmin      bool
	CreatedAt    time.Time
	Permissions  Permissions
	Theme        string
}

type Permissions struct {
	CanUpload         bool
	CanDownload       bool
	CanEditMetadata   bool
	CanManageLibrary  bool
}

func (p Permissions) CanAdmin(u User) bool {
	return u.IsAdmin
}

type Library struct {
	ID        int64
	Name      string
	Icon      string
	Watch     bool
	CreatedAt time.Time
	Paths     []LibraryPath
	BookCount int
}

type LibraryPath struct {
	ID        int64
	LibraryID int64
	Path      string
}

type Book struct {
	ID            int64
	LibraryID     int64
	LibraryPathID *int64
	FileName      string
	FileSubPath   string
	Format        string
	FileSize      int64
	FileHash      *string
	Deleted       bool
	AddedOn       time.Time
	Metadata      BookMetadata
	Authors       []string
	Categories    []string
	LibraryName   string
}

type BookMetadata struct {
	BookID        int64
	Title         *string
	Subtitle      *string
	Description   *string
	Publisher     *string
	PublishedDate *time.Time
	ISBN10        *string
	ISBN13        *string
	PageCount     *int
	Language      *string
	SeriesName    *string
	SeriesNumber  *float64
	SeriesTotal   *int
	CoverPath     *string
	Rating        *float64
}

func (m BookMetadata) DisplayTitle(fallback string) string {
	if m.Title != nil && *m.Title != "" {
		return *m.Title
	}
	return fallback
}

type Shelf struct {
	ID        int64
	UserID    int64
	Name      string
	Icon      string
	BookCount int
}

type MagicShelf struct {
	ID     int64
	UserID int64
	Name   string
	Icon   string
	Rules  MagicRules
}

type MagicRules struct {
	Author     string `json:"author,omitempty"`
	Category   string `json:"category,omitempty"`
	Series     string `json:"series,omitempty"`
	Format     string `json:"format,omitempty"`
	Status     string `json:"status,omitempty"`
	LibraryID  int64  `json:"library_id,omitempty"`
	Query      string `json:"query,omitempty"`
}

func (r MagicRules) JSON() json.RawMessage {
	b, _ := json.Marshal(r)
	return b
}

type Progress struct {
	UserID    int64
	BookID    int64
	Percent   float64
	Position  json.RawMessage
	Status    string
	UpdatedAt time.Time
}

// Annotation kinds and colors for private multi-format highlights/notes/bookmarks.
const (
	AnnotationKindHighlight = "highlight"
	AnnotationKindNote      = "note"
	AnnotationKindBookmark  = "bookmark"

	// MaxAnnotationsPerBook is the soft cap per user per book (PRD Q5).
	MaxAnnotationsPerBook = 5000
	MaxNoteLen            = 10000
	MaxQuoteLen           = 8000
	MaxTags               = 32
	MaxTagLen             = 40
)

// AllowedAnnotationColors are stored as names; UI maps to theme tokens.
var AllowedAnnotationColors = map[string]bool{
	"yellow": true,
	"green":  true,
	"blue":   true,
	"pink":   true,
	"purple": true,
}

type Annotation struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id"`
	BookID    int64           `json:"book_id"`
	Kind      string          `json:"kind"`
	Color     *string         `json:"color,omitempty"`
	Quote     string          `json:"quote"`
	Note      string          `json:"note"`
	Tags      []string        `json:"tags"`
	Anchor    json.RawMessage `json:"anchor"`
	SortKey   string          `json:"sort_key"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	// Joined for global list / export
	BookTitle  string `json:"book_title,omitempty"`
	BookFormat string `json:"book_format,omitempty"`
	Authors    string `json:"authors,omitempty"`
}

type AnnotationFilter struct {
	BookID    int64
	LibraryID int64
	Format    string
	Kind      string
	Color     string
	Tag       string
	Query     string
	Limit     int
	Offset    int
}

type BookdropFile struct {
	ID            int64
	Path          string
	FileName      string
	Format        *string
	Status        string
	MetadataJSON  json.RawMessage
	CoverPath     *string
	ErrorMessage  *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// parsed convenience
	Title   string
	Authors string
}

type OPDSUser struct {
	ID           int64
	Username     string
	PasswordHash string
	UserID       *int64
}

type ExtractedMetadata struct {
	Title         string
	Subtitle      string
	Description   string
	Publisher     string
	PublishedDate string
	ISBN10        string
	ISBN13        string
	PageCount     int
	Language      string
	SeriesName    string
	SeriesNumber  float64
	Authors       []string
	Categories    []string
	CoverData     []byte
	CoverExt      string
}

type DashboardData struct {
	BookCount      int
	LibraryCount   int
	InProgress     []Book
	RecentlyAdded  []Book
	BookdropReady  int
}
