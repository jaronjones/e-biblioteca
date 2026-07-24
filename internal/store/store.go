package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jjones/e-biblioteca/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, username, hash, displayName string, admin bool, perms models.Permissions) (*models.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var u models.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, display_name, is_admin)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, password_hash, display_name, email, is_admin, created_at
	`, username, hash, displayName, admin).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.IsAdmin, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if admin {
		perms = models.Permissions{
			CanUpload: true, CanDownload: true, CanEditMetadata: true, CanManageLibrary: true,
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_permissions (user_id, can_upload, can_download, can_edit_metadata, can_manage_library)
		VALUES ($1,$2,$3,$4,$5)
	`, u.ID, perms.CanUpload, perms.CanDownload, perms.CanEditMetadata, perms.CanManageLibrary)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_settings (user_id, theme) VALUES ($1, 'dark')`, u.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	u.Permissions = perms
	u.Theme = "dark"
	return &u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	return s.scanUser(ctx, `
		SELECT u.id, u.username, u.password_hash, u.display_name, u.email, u.is_admin, u.created_at,
		       COALESCE(p.can_upload,false), COALESCE(p.can_download,true),
		       COALESCE(p.can_edit_metadata,false), COALESCE(p.can_manage_library,false),
		       COALESCE(s.theme,'dark')
		FROM users u
		LEFT JOIN user_permissions p ON p.user_id = u.id
		LEFT JOIN user_settings s ON s.user_id = u.id
		WHERE u.username = $1
	`, username)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	return s.scanUser(ctx, `
		SELECT u.id, u.username, u.password_hash, u.display_name, u.email, u.is_admin, u.created_at,
		       COALESCE(p.can_upload,false), COALESCE(p.can_download,true),
		       COALESCE(p.can_edit_metadata,false), COALESCE(p.can_manage_library,false),
		       COALESCE(s.theme,'dark')
		FROM users u
		LEFT JOIN user_permissions p ON p.user_id = u.id
		LEFT JOIN user_settings s ON s.user_id = u.id
		WHERE u.id = $1
	`, id)
}

func (s *Store) scanUser(ctx context.Context, q string, arg any) (*models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.IsAdmin, &u.CreatedAt,
		&u.Permissions.CanUpload, &u.Permissions.CanDownload,
		&u.Permissions.CanEditMetadata, &u.Permissions.CanManageLibrary,
		&u.Theme,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.password_hash, u.display_name, u.email, u.is_admin, u.created_at,
		       COALESCE(p.can_upload,false), COALESCE(p.can_download,true),
		       COALESCE(p.can_edit_metadata,false), COALESCE(p.can_manage_library,false),
		       COALESCE(s.theme,'dark')
		FROM users u
		LEFT JOIN user_permissions p ON p.user_id = u.id
		LEFT JOIN user_settings s ON s.user_id = u.id
		ORDER BY u.username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.IsAdmin, &u.CreatedAt,
			&u.Permissions.CanUpload, &u.Permissions.CanDownload,
			&u.Permissions.CanEditMetadata, &u.Permissions.CanManageLibrary,
			&u.Theme,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUserTheme(ctx context.Context, userID int64, theme string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, theme) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET theme = EXCLUDED.theme
	`, userID, theme)
	return err
}

func (s *Store) UpdateUserPermissions(ctx context.Context, userID int64, perms models.Permissions, isAdmin bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE users SET is_admin=$2 WHERE id=$1`, userID, isAdmin)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_permissions (user_id, can_upload, can_download, can_edit_metadata, can_manage_library)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id) DO UPDATE SET
		  can_upload=EXCLUDED.can_upload,
		  can_download=EXCLUDED.can_download,
		  can_edit_metadata=EXCLUDED.can_edit_metadata,
		  can_manage_library=EXCLUDED.can_manage_library
	`, userID, perms.CanUpload, perms.CanDownload, perms.CanEditMetadata, perms.CanManageLibrary)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateLibrary(ctx context.Context, name, icon string, watch bool, paths []string) (*models.Library, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var lib models.Library
	err = tx.QueryRow(ctx, `
		INSERT INTO libraries (name, icon, watch) VALUES ($1,$2,$3)
		RETURNING id, name, icon, watch, created_at
	`, name, icon, watch).Scan(&lib.ID, &lib.Name, &lib.Icon, &lib.Watch, &lib.CreatedAt)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var lp models.LibraryPath
		err = tx.QueryRow(ctx, `
			INSERT INTO library_paths (library_id, path) VALUES ($1,$2)
			RETURNING id, library_id, path
		`, lib.ID, p).Scan(&lp.ID, &lp.LibraryID, &lp.Path)
		if err != nil {
			return nil, err
		}
		lib.Paths = append(lib.Paths, lp)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &lib, nil
}

func (s *Store) ListLibraries(ctx context.Context) ([]models.Library, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.name, l.icon, l.watch, l.created_at,
		       (SELECT COUNT(*) FROM books b WHERE b.library_id=l.id AND b.deleted=false)
		FROM libraries l ORDER BY l.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var libs []models.Library
	for rows.Next() {
		var l models.Library
		if err := rows.Scan(&l.ID, &l.Name, &l.Icon, &l.Watch, &l.CreatedAt, &l.BookCount); err != nil {
			return nil, err
		}
		paths, err := s.libraryPaths(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		l.Paths = paths
		libs = append(libs, l)
	}
	return libs, rows.Err()
}

func (s *Store) GetLibrary(ctx context.Context, id int64) (*models.Library, error) {
	var l models.Library
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, icon, watch, created_at,
		       (SELECT COUNT(*) FROM books b WHERE b.library_id=$1 AND b.deleted=false)
		FROM libraries WHERE id=$1
	`, id).Scan(&l.ID, &l.Name, &l.Icon, &l.Watch, &l.CreatedAt, &l.BookCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	paths, err := s.libraryPaths(ctx, id)
	if err != nil {
		return nil, err
	}
	l.Paths = paths
	return &l, nil
}

func (s *Store) libraryPaths(ctx context.Context, libraryID int64) ([]models.LibraryPath, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, library_id, path FROM library_paths WHERE library_id=$1`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LibraryPath
	for rows.Next() {
		var p models.LibraryPath
		if err := rows.Scan(&p.ID, &p.LibraryID, &p.Path); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteLibrary(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM libraries WHERE id=$1`, id)
	return err
}

type BookFilter struct {
	Query     string
	LibraryID int64
	Format    string
	Author    string
	Category  string
	Series    string
	Status    string
	UserID    int64
	Limit     int
	Offset    int
}

func (s *Store) ListBooks(ctx context.Context, f BookFilter) ([]models.Book, int, error) {
	if f.Limit <= 0 {
		f.Limit = 48
	}
	args := []any{}
	where := []string{"b.deleted = false"}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.LibraryID > 0 {
		where = append(where, "b.library_id = "+arg(f.LibraryID))
	}
	if f.Format != "" {
		where = append(where, "b.format = "+arg(f.Format))
	}
	if f.Query != "" {
		where = append(where, "(m.search_vector @@ plainto_tsquery('english', "+arg(f.Query)+") OR m.title ILIKE "+arg("%"+f.Query+"%")+")")
	}
	if f.Author != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM book_authors ba JOIN authors a ON a.id=ba.author_id
			WHERE ba.book_id=b.id AND a.name ILIKE `+arg("%"+f.Author+"%")+`)`)
	}
	if f.Category != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM book_categories bc JOIN categories c ON c.id=bc.category_id
			WHERE bc.book_id=b.id AND c.name ILIKE `+arg("%"+f.Category+"%")+`)`)
	}
	if f.Series != "" {
		where = append(where, "m.series_name ILIKE "+arg("%"+f.Series+"%"))
	}
	if f.Status != "" && f.UserID > 0 {
		where = append(where, `EXISTS (
			SELECT 1 FROM user_book_progress ubp
			WHERE ubp.book_id=b.id AND ubp.user_id=`+arg(f.UserID)+` AND ubp.status=`+arg(f.Status)+`)`)
	}

	w := strings.Join(where, " AND ")
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM books b LEFT JOIN book_metadata m ON m.book_id=b.id WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	limitP := fmt.Sprintf("$%d", len(args)-1)
	offsetP := fmt.Sprintf("$%d", len(args))

	q := `
		SELECT b.id, b.library_id, b.library_path_id, b.file_name, b.file_sub_path, b.format,
		       b.file_size, b.file_hash, b.deleted, b.added_on,
		       m.title, m.subtitle, m.description, m.publisher, m.published_date,
		       m.isbn10, m.isbn13, m.page_count, m.language, m.series_name, m.series_number,
		       m.series_total, m.cover_path, m.rating,
		       l.name
		FROM books b
		LEFT JOIN book_metadata m ON m.book_id = b.id
		JOIN libraries l ON l.id = b.library_id
		WHERE ` + w + `
		ORDER BY COALESCE(m.title, b.file_name)
		LIMIT ` + limitP + ` OFFSET ` + offsetP

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, 0, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range books {
		authors, cats, err := s.bookTaxonomy(ctx, books[i].ID)
		if err != nil {
			return nil, 0, err
		}
		books[i].Authors = authors
		books[i].Categories = cats
	}
	return books, total, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanBook(row scannable) (models.Book, error) {
	var b models.Book
	var m models.BookMetadata
	err := row.Scan(
		&b.ID, &b.LibraryID, &b.LibraryPathID, &b.FileName, &b.FileSubPath, &b.Format,
		&b.FileSize, &b.FileHash, &b.Deleted, &b.AddedOn,
		&m.Title, &m.Subtitle, &m.Description, &m.Publisher, &m.PublishedDate,
		&m.ISBN10, &m.ISBN13, &m.PageCount, &m.Language, &m.SeriesName, &m.SeriesNumber,
		&m.SeriesTotal, &m.CoverPath, &m.Rating,
		&b.LibraryName,
	)
	m.BookID = b.ID
	b.Metadata = m
	return b, err
}

func (s *Store) GetBook(ctx context.Context, id int64) (*models.Book, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT b.id, b.library_id, b.library_path_id, b.file_name, b.file_sub_path, b.format,
		       b.file_size, b.file_hash, b.deleted, b.added_on,
		       m.title, m.subtitle, m.description, m.publisher, m.published_date,
		       m.isbn10, m.isbn13, m.page_count, m.language, m.series_name, m.series_number,
		       m.series_total, m.cover_path, m.rating,
		       l.name
		FROM books b
		LEFT JOIN book_metadata m ON m.book_id = b.id
		JOIN libraries l ON l.id = b.library_id
		WHERE b.id=$1 AND b.deleted=false
	`, id)
	b, err := scanBook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	authors, cats, err := s.bookTaxonomy(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	b.Authors = authors
	b.Categories = cats
	return &b, nil
}

func (s *Store) bookTaxonomy(ctx context.Context, bookID int64) ([]string, []string, error) {
	arows, err := s.pool.Query(ctx, `
		SELECT a.name FROM authors a
		JOIN book_authors ba ON ba.author_id=a.id WHERE ba.book_id=$1 ORDER BY a.name
	`, bookID)
	if err != nil {
		return nil, nil, err
	}
	defer arows.Close()
	var authors []string
	for arows.Next() {
		var n string
		if err := arows.Scan(&n); err != nil {
			return nil, nil, err
		}
		authors = append(authors, n)
	}
	crows, err := s.pool.Query(ctx, `
		SELECT c.name FROM categories c
		JOIN book_categories bc ON bc.category_id=c.id WHERE bc.book_id=$1 ORDER BY c.name
	`, bookID)
	if err != nil {
		return nil, nil, err
	}
	defer crows.Close()
	var cats []string
	for crows.Next() {
		var n string
		if err := crows.Scan(&n); err != nil {
			return nil, nil, err
		}
		cats = append(cats, n)
	}
	return authors, cats, nil
}

func (s *Store) UpsertBook(ctx context.Context, book models.Book, meta models.ExtractedMetadata) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO books (library_id, library_path_id, file_name, file_sub_path, format, file_size, file_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (library_id, file_sub_path, file_name) DO UPDATE SET
		  format=EXCLUDED.format, file_size=EXCLUDED.file_size, file_hash=EXCLUDED.file_hash, deleted=false, deleted_at=NULL
		RETURNING id
	`, book.LibraryID, book.LibraryPathID, book.FileName, book.FileSubPath, book.Format, book.FileSize, book.FileHash).Scan(&id)
	if err != nil {
		return 0, err
	}

	title := meta.Title
	if title == "" {
		title = book.FileName
	}
	var pubDate *time.Time
	if meta.PublishedDate != "" {
		if t, e := time.Parse("2006-01-02", meta.PublishedDate); e == nil {
			pubDate = &t
		}
	}
	var pageCount *int
	if meta.PageCount > 0 {
		pageCount = &meta.PageCount
	}
	var seriesNum *float64
	if meta.SeriesNumber > 0 {
		seriesNum = &meta.SeriesNumber
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO book_metadata (book_id, title, subtitle, description, publisher, published_date,
		  isbn10, isbn13, page_count, language, series_name, series_number, cover_path)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,''),NULLIF($8,''),$9,NULLIF($10,''),NULLIF($11,''),$12,$13)
		ON CONFLICT (book_id) DO UPDATE SET
		  title=COALESCE(NULLIF(EXCLUDED.title,''), book_metadata.title),
		  subtitle=COALESCE(EXCLUDED.subtitle, book_metadata.subtitle),
		  description=COALESCE(EXCLUDED.description, book_metadata.description),
		  publisher=COALESCE(EXCLUDED.publisher, book_metadata.publisher),
		  published_date=COALESCE(EXCLUDED.published_date, book_metadata.published_date),
		  isbn10=COALESCE(EXCLUDED.isbn10, book_metadata.isbn10),
		  isbn13=COALESCE(EXCLUDED.isbn13, book_metadata.isbn13),
		  page_count=COALESCE(EXCLUDED.page_count, book_metadata.page_count),
		  language=COALESCE(EXCLUDED.language, book_metadata.language),
		  series_name=COALESCE(EXCLUDED.series_name, book_metadata.series_name),
		  series_number=COALESCE(EXCLUDED.series_number, book_metadata.series_number),
		  cover_path=COALESCE(EXCLUDED.cover_path, book_metadata.cover_path)
	`, id, title, meta.Subtitle, meta.Description, meta.Publisher, pubDate,
		meta.ISBN10, meta.ISBN13, pageCount, meta.Language, meta.SeriesName, seriesNum, book.Metadata.CoverPath)
	if err != nil {
		return 0, err
	}

	// authors
	_, _ = tx.Exec(ctx, `DELETE FROM book_authors WHERE book_id=$1`, id)
	for _, name := range meta.Authors {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var aid int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO authors (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id
		`, name).Scan(&aid); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO book_authors (book_id, author_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, aid); err != nil {
			return 0, err
		}
	}
	_, _ = tx.Exec(ctx, `DELETE FROM book_categories WHERE book_id=$1`, id)
	for _, name := range meta.Categories {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var cid int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO categories (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id
		`, name).Scan(&cid); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO book_categories (book_id, category_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, cid); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) UpdateBookMetadata(ctx context.Context, bookID int64, title, subtitle, description, publisher, series, isbn13 string, authors, categories []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		UPDATE book_metadata SET title=$2, subtitle=NULLIF($3,''), description=NULLIF($4,''),
		  publisher=NULLIF($5,''), series_name=NULLIF($6,''), isbn13=NULLIF($7,'')
		WHERE book_id=$1
	`, bookID, title, subtitle, description, publisher, series, isbn13)
	if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `DELETE FROM book_authors WHERE book_id=$1`, bookID)
	for _, name := range authors {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var aid int64
		if err := tx.QueryRow(ctx, `INSERT INTO authors (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`, name).Scan(&aid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO book_authors (book_id, author_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, bookID, aid); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(ctx, `DELETE FROM book_categories WHERE book_id=$1`, bookID)
	for _, name := range categories {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var cid int64
		if err := tx.QueryRow(ctx, `INSERT INTO categories (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`, name).Scan(&cid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO book_categories (book_id, category_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, bookID, cid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) SetBookCover(ctx context.Context, bookID int64, coverPath string) error {
	_, err := s.pool.Exec(ctx, `UPDATE book_metadata SET cover_path=$2 WHERE book_id=$1`, bookID, coverPath)
	return err
}

func (s *Store) SoftDeleteBook(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE books SET deleted=true, deleted_at=NOW() WHERE id=$1`, id)
	return err
}

func (s *Store) Dashboard(ctx context.Context, userID int64) (models.DashboardData, error) {
	var d models.DashboardData
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM books WHERE deleted=false`).Scan(&d.BookCount)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM libraries`).Scan(&d.LibraryCount)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bookdrop_files WHERE status='ready'`).Scan(&d.BookdropReady)

	recent, _, err := s.ListBooks(ctx, BookFilter{Limit: 8, Offset: 0})
	if err == nil {
		d.RecentlyAdded = recent
	}
	// in progress
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.library_id, b.library_path_id, b.file_name, b.file_sub_path, b.format,
		       b.file_size, b.file_hash, b.deleted, b.added_on,
		       m.title, m.subtitle, m.description, m.publisher, m.published_date,
		       m.isbn10, m.isbn13, m.page_count, m.language, m.series_name, m.series_number,
		       m.series_total, m.cover_path, m.rating, l.name
		FROM user_book_progress ubp
		JOIN books b ON b.id=ubp.book_id AND b.deleted=false
		LEFT JOIN book_metadata m ON m.book_id=b.id
		JOIN libraries l ON l.id=b.library_id
		WHERE ubp.user_id=$1 AND ubp.status='reading'
		ORDER BY ubp.updated_at DESC LIMIT 8
	`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			b, err := scanBook(rows)
			if err != nil {
				break
			}
			d.InProgress = append(d.InProgress, b)
		}
	}
	return d, nil
}

// Shelves
func (s *Store) ListShelves(ctx context.Context, userID int64) ([]models.Shelf, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sh.id, sh.user_id, sh.name, sh.icon,
		       (SELECT COUNT(*) FROM shelf_books sb WHERE sb.shelf_id=sh.id)
		FROM shelves sh WHERE sh.user_id=$1 ORDER BY sh.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Shelf
	for rows.Next() {
		var sh models.Shelf
		if err := rows.Scan(&sh.ID, &sh.UserID, &sh.Name, &sh.Icon, &sh.BookCount); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (s *Store) CreateShelf(ctx context.Context, userID int64, name, icon string) (*models.Shelf, error) {
	var sh models.Shelf
	err := s.pool.QueryRow(ctx, `
		INSERT INTO shelves (user_id, name, icon) VALUES ($1,$2,$3)
		RETURNING id, user_id, name, icon
	`, userID, name, icon).Scan(&sh.ID, &sh.UserID, &sh.Name, &sh.Icon)
	return &sh, err
}

func (s *Store) AddToShelf(ctx context.Context, shelfID, bookID int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO shelf_books (shelf_id, book_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, shelfID, bookID)
	return err
}

func (s *Store) RemoveFromShelf(ctx context.Context, shelfID, bookID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM shelf_books WHERE shelf_id=$1 AND book_id=$2`, shelfID, bookID)
	return err
}

func (s *Store) DeleteShelf(ctx context.Context, id, userID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM shelves WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (s *Store) ShelfBooks(ctx context.Context, shelfID int64) ([]models.Book, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.library_id, b.library_path_id, b.file_name, b.file_sub_path, b.format,
		       b.file_size, b.file_hash, b.deleted, b.added_on,
		       m.title, m.subtitle, m.description, m.publisher, m.published_date,
		       m.isbn10, m.isbn13, m.page_count, m.language, m.series_name, m.series_number,
		       m.series_total, m.cover_path, m.rating, l.name
		FROM shelf_books sb
		JOIN books b ON b.id=sb.book_id AND b.deleted=false
		LEFT JOIN book_metadata m ON m.book_id=b.id
		JOIN libraries l ON l.id=b.library_id
		WHERE sb.shelf_id=$1
		ORDER BY COALESCE(m.title, b.file_name)
	`, shelfID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []models.Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

func (s *Store) ListMagicShelves(ctx context.Context, userID int64) ([]models.MagicShelf, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, name, icon, rules FROM magic_shelves WHERE user_id=$1 ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.MagicShelf
	for rows.Next() {
		var ms models.MagicShelf
		var raw []byte
		if err := rows.Scan(&ms.ID, &ms.UserID, &ms.Name, &ms.Icon, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &ms.Rules)
		out = append(out, ms)
	}
	return out, rows.Err()
}

func (s *Store) CreateMagicShelf(ctx context.Context, userID int64, name, icon string, rules models.MagicRules) (*models.MagicShelf, error) {
	var ms models.MagicShelf
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO magic_shelves (user_id, name, icon, rules) VALUES ($1,$2,$3,$4)
		RETURNING id, user_id, name, icon, rules
	`, userID, name, icon, rules.JSON()).Scan(&ms.ID, &ms.UserID, &ms.Name, &ms.Icon, &raw)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &ms.Rules)
	return &ms, nil
}

func (s *Store) GetMagicShelf(ctx context.Context, id, userID int64) (*models.MagicShelf, error) {
	var ms models.MagicShelf
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, name, icon, rules FROM magic_shelves WHERE id=$1 AND user_id=$2`, id, userID).
		Scan(&ms.ID, &ms.UserID, &ms.Name, &ms.Icon, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &ms.Rules)
	return &ms, nil
}

func (s *Store) DeleteMagicShelf(ctx context.Context, id, userID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM magic_shelves WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (s *Store) SaveProgress(ctx context.Context, userID, bookID int64, percent float64, position json.RawMessage, status string) error {
	if status == "" {
		if percent >= 98 {
			status = "finished"
		} else if percent > 0 {
			status = "reading"
		} else {
			status = "unread"
		}
	}
	if len(position) == 0 {
		position = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_book_progress (user_id, book_id, percent, position, status, updated_at)
		VALUES ($1,$2,$3,$4,$5,NOW())
		ON CONFLICT (user_id, book_id) DO UPDATE SET
		  percent=EXCLUDED.percent, position=EXCLUDED.position, status=EXCLUDED.status, updated_at=NOW()
	`, userID, bookID, percent, position, status)
	return err
}

func (s *Store) GetProgress(ctx context.Context, userID, bookID int64) (*models.Progress, error) {
	var p models.Progress
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, book_id, percent, position, status, updated_at
		FROM user_book_progress WHERE user_id=$1 AND book_id=$2
	`, userID, bookID).Scan(&p.UserID, &p.BookID, &p.Percent, &p.Position, &p.Status, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &models.Progress{UserID: userID, BookID: bookID, Status: "unread", Position: json.RawMessage(`{}`)}, nil
	}
	return &p, err
}

// Bookdrop
func (s *Store) UpsertBookdrop(ctx context.Context, path, fileName, format, status string, meta json.RawMessage, cover *string, errMsg *string) error {
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bookdrop_files (path, file_name, format, status, metadata_json, cover_path, error_message, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
		ON CONFLICT (path) DO UPDATE SET
		  file_name=EXCLUDED.file_name, format=EXCLUDED.format, status=EXCLUDED.status,
		  metadata_json=EXCLUDED.metadata_json, cover_path=EXCLUDED.cover_path,
		  error_message=EXCLUDED.error_message, updated_at=NOW()
	`, path, fileName, nullStr(format), status, meta, cover, errMsg)
	return err
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) ListBookdrop(ctx context.Context) ([]models.BookdropFile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, path, file_name, format, status, metadata_json, cover_path, error_message, created_at, updated_at
		FROM bookdrop_files WHERE status IN ('pending','ready','error')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.BookdropFile
	for rows.Next() {
		var f models.BookdropFile
		if err := rows.Scan(&f.ID, &f.Path, &f.FileName, &f.Format, &f.Status, &f.MetadataJSON, &f.CoverPath, &f.ErrorMessage, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		var m map[string]any
		_ = json.Unmarshal(f.MetadataJSON, &m)
		if t, ok := m["title"].(string); ok {
			f.Title = t
		}
		if a, ok := m["authors"].(string); ok {
			f.Authors = a
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetBookdrop(ctx context.Context, id int64) (*models.BookdropFile, error) {
	var f models.BookdropFile
	err := s.pool.QueryRow(ctx, `
		SELECT id, path, file_name, format, status, metadata_json, cover_path, error_message, created_at, updated_at
		FROM bookdrop_files WHERE id=$1
	`, id).Scan(&f.ID, &f.Path, &f.FileName, &f.Format, &f.Status, &f.MetadataJSON, &f.CoverPath, &f.ErrorMessage, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

func (s *Store) UpdateBookdropStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE bookdrop_files SET status=$2, updated_at=NOW() WHERE id=$1`, id, status)
	return err
}

func (s *Store) DeleteBookdrop(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM bookdrop_files WHERE id=$1`, id)
	return err
}

func (s *Store) CreateOPDSUser(ctx context.Context, username, hash string, userID *int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO opds_users (username, password_hash, user_id) VALUES ($1,$2,$3)`, username, hash, userID)
	return err
}

func (s *Store) GetOPDSUser(ctx context.Context, username string) (*models.OPDSUser, error) {
	var u models.OPDSUser
	err := s.pool.QueryRow(ctx, `SELECT id, username, password_hash, user_id FROM opds_users WHERE username=$1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) ListOPDSUsers(ctx context.Context) ([]models.OPDSUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, username, password_hash, user_id FROM opds_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OPDSUser
	for rows.Next() {
		var u models.OPDSUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.UserID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOPDSUser(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM opds_users WHERE id=$1`, id)
	return err
}

func (s *Store) FindBookByHash(ctx context.Context, hash string) (*models.Book, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM books WHERE file_hash=$1 AND deleted=false LIMIT 1`, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetBook(ctx, id)
}
