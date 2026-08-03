package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/a-h/templ"
	"github.com/jjones/e-biblioteca/components"
	"github.com/jjones/e-biblioteca/internal/auth"
	"github.com/jjones/e-biblioteca/internal/config"
	"github.com/jjones/e-biblioteca/internal/models"
	"github.com/jjones/e-biblioteca/internal/service/bookdrop"
	"github.com/jjones/e-biblioteca/internal/service/metadata"
	"github.com/jjones/e-biblioteca/internal/service/scanner"
	"github.com/jjones/e-biblioteca/internal/service/theme"
	"github.com/jjones/e-biblioteca/internal/store"
)

type App struct {
	Cfg      config.Config
	Store    *store.Store
	Auth     *auth.Manager
	Scanner  *scanner.Scanner
	Bookdrop *bookdrop.Service
}

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(a.Auth.Sessions.LoadAndSave)
	r.Use(a.loadUser)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	r.Get("/setup", a.setupGet)
	r.Post("/setup", a.setupPost)
	r.Get("/login", a.loginGet)
	r.Post("/login", a.loginPost)
	r.Post("/logout", a.logout)

	r.Group(func(pr chi.Router) {
		pr.Use(a.requireAuth)
		pr.Get("/", a.dashboard)
		pr.Get("/books", a.booksList)
		pr.Get("/books/{id}", a.bookDetail)
		pr.Get("/books/{id}/edit", a.bookEditGet)
		pr.Post("/books/{id}/edit", a.bookEditPost)
		pr.Post("/books/{id}/delete", a.bookDelete)
		pr.Get("/books/{id}/download", a.bookDownload)
		pr.Get("/covers/{id}", a.coverGet)
		pr.Post("/books/{id}/lookup", a.bookLookup)
		pr.Post("/books/{id}/apply-lookup", a.bookApplyLookup)

		pr.Get("/libraries", a.librariesList)
		pr.Post("/libraries", a.librariesCreate)
		pr.Post("/libraries/{id}/scan", a.librariesScan)
		pr.Post("/libraries/{id}/delete", a.librariesDelete)

		pr.Get("/shelves", a.shelvesList)
		pr.Post("/shelves", a.shelvesCreate)
		pr.Get("/shelves/{id}", a.shelfDetail)
		pr.Post("/shelves/{id}/delete", a.shelfDelete)
		pr.Post("/shelves/{id}/books/{bookID}", a.shelfAddBook)
		pr.Post("/shelves/{id}/books/{bookID}/remove", a.shelfRemoveBook)

		pr.Get("/magic-shelves", a.magicList)
		pr.Post("/magic-shelves", a.magicCreate)
		pr.Get("/magic-shelves/{id}", a.magicDetail)
		pr.Post("/magic-shelves/{id}/delete", a.magicDelete)

		pr.Get("/bookdrop", a.bookdropList)
		pr.Post("/bookdrop/{id}/import", a.bookdropImport)
		pr.Post("/bookdrop/{id}/reject", a.bookdropReject)
		pr.Post("/upload", a.upload)

		pr.Get("/read/{id}", a.readBook)
		pr.Get("/read/{id}/pages/{n}", a.cbzPage)
		pr.Get("/stream/{id}", a.streamFile)
		pr.Post("/progress/{id}", a.saveProgress)
		pr.Get("/api/progress/{id}", a.getProgress)

		pr.Get("/annotations", a.annotationsPage)
		pr.Get("/annotations/export", a.exportAnnotations)
		pr.Get("/books/{id}/annotations/export", a.exportAnnotations)
		pr.Get("/api/books/{id}/annotations", a.listBookAnnotationsAPI)
		pr.Post("/api/books/{id}/annotations", a.createBookAnnotationAPI)
		pr.Patch("/api/annotations/{id}", a.patchAnnotationAPI)
		pr.Delete("/api/annotations/{id}", a.deleteAnnotationAPI)

		pr.Get("/settings", a.settings)
		pr.Post("/settings/theme", a.setTheme)
		pr.Post("/settings/users", a.createUser)
		pr.Post("/settings/users/{id}/perms", a.updateUserPerms)
		pr.Post("/settings/opds", a.createOPDSUser)
		pr.Post("/settings/opds/{id}/delete", a.deleteOPDSUser)
	})

	// OPDS
	r.Route("/opds", func(or chi.Router) {
		or.Use(a.opdsBasicAuth)
		or.Get("/", a.opdsRoot)
		or.Get("/catalog", a.opdsCatalog)
		or.Get("/download/{id}", a.bookDownload)
	})

	return r
}

func (a *App) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := a.Auth.UserID(r.Context()); ok {
			if u, err := a.Store.GetUserByID(r.Context(), id); err == nil {
				r = r.WithContext(auth.WithUser(r.Context(), u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := a.Store.UserCount(r.Context())
		if n == 0 {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) currentUser(r *http.Request) *models.User {
	u, _ := auth.UserFromContext(r.Context())
	return u
}

func (a *App) themeFor(r *http.Request) string {
	if u := a.currentUser(r); u != nil {
		return theme.Normalize(u.Theme)
	}
	if c, err := r.Cookie("theme"); err == nil {
		return theme.Normalize(c.Value)
	}
	return theme.Default
}

func (a *App) render(w http.ResponseWriter, r *http.Request, title, active string, body templ.Component) {
	u := a.currentUser(r)
	nav := components.NavData{Active: active, Theme: a.themeFor(r), Themes: theme.Names}
	if u != nil {
		nav.User = u.DisplayName
		if nav.User == "" {
			nav.User = u.Username
		}
		nav.IsAdmin = u.IsAdmin
	}
	// bookdrop badge
	if files, err := a.Store.ListBookdrop(r.Context()); err == nil {
		for _, f := range files {
			if f.Status == "ready" {
				nav.BookdropCount++
			}
		}
	}
	components.Layout(title, nav, body).Render(r.Context(), w)
}

func (a *App) setupGet(w http.ResponseWriter, r *http.Request) {
	n, _ := a.Store.UserCount(r.Context())
	if n > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	a.render(w, r, "Setup", "", components.SetupPage(""))
}

func (a *App) setupPost(w http.ResponseWriter, r *http.Request) {
	n, _ := a.Store.UserCount(r.Context())
	if n > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	display := strings.TrimSpace(r.FormValue("display_name"))
	if username == "" || len(password) < 8 {
		a.render(w, r, "Setup", "", components.SetupPage("Username required and password must be at least 8 characters."))
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		a.render(w, r, "Setup", "", components.SetupPage("Could not hash password."))
		return
	}
	if display == "" {
		display = username
	}
	u, err := a.Store.CreateUser(r.Context(), username, hash, display, true, models.Permissions{})
	if err != nil {
		a.render(w, r, "Setup", "", components.SetupPage("Could not create admin: "+err.Error()))
		return
	}
	a.Auth.Login(r.Context(), r, w, u.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) loginGet(w http.ResponseWriter, r *http.Request) {
	n, _ := a.Store.UserCount(r.Context())
	if n == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, r, "Login", "", components.LoginPage(""))
}

func (a *App) loginPost(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	u, err := a.Store.GetUserByUsername(r.Context(), username)
	if err != nil || !auth.CheckPassword(u.PasswordHash, password) {
		a.render(w, r, "Login", "", components.LoginPage("Invalid username or password."))
		return
	}
	a.Auth.Login(r.Context(), r, w, u.ID)
	http.SetCookie(w, &http.Cookie{Name: "theme", Value: u.Theme, Path: "/", MaxAge: 365 * 24 * 3600})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	a.Auth.Logout(r.Context())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	d, _ := a.Store.Dashboard(r.Context(), u.ID)
	a.render(w, r, "Dashboard", "dashboard", components.DashboardPage(d))
}

func (a *App) booksList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	format := r.URL.Query().Get("format")
	libID, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 48
	books, total, err := a.Store.ListBooks(r.Context(), store.BookFilter{
		Query: q, Format: format, LibraryID: libID, Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	libs, _ := a.Store.ListLibraries(r.Context())
	pages := (total + limit - 1) / limit
	body := components.BooksPage(books, libs, q, format, libID, page, pages, total)
	if r.Header.Get("HX-Request") == "true" {
		components.BookGrid(books).Render(r.Context(), w)
		return
	}
	a.render(w, r, "Books", "books", body)
}

func (a *App) bookDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := a.currentUser(r)
	prog, _ := a.Store.GetProgress(r.Context(), u.ID, id)
	shelves, _ := a.Store.ListShelves(r.Context(), u.ID)
	annCount, _ := a.Store.CountAnnotationsForBook(r.Context(), u.ID, id)
	a.render(w, r, book.Metadata.DisplayTitle(book.FileName), "books", components.BookDetailPage(*book, prog, shelves, u, annCount))
}

func (a *App) bookEditGet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, r, "Edit metadata", "books", components.BookEditPage(*book, ""))
}

func (a *App) bookEditPost(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanEditMetadata }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	authors := splitCSV(r.FormValue("authors"))
	cats := splitCSV(r.FormValue("categories"))
	if err := a.Store.UpdateBookMetadata(r.Context(), id,
		r.FormValue("title"), r.FormValue("subtitle"), r.FormValue("description"),
		r.FormValue("publisher"), r.FormValue("series"), r.FormValue("isbn13"),
		authors, cats,
	); err != nil {
		book, _ := a.Store.GetBook(r.Context(), id)
		a.render(w, r, "Edit metadata", "books", components.BookEditPage(*book, err.Error()))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/books/%d", id), http.StatusSeeOther)
}

func splitCSV(s string) []string {
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

func (a *App) bookDelete(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanManageLibrary }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = a.Store.SoftDeleteBook(r.Context(), id)
	http.Redirect(w, r, "/books", http.StatusSeeOther)
}

func (a *App) bookDownload(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	// OPDS may use basic auth without session user
	if u != nil {
		if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanDownload }); err != nil {
			http.Error(w, "forbidden", 403)
			return
		}
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := a.Scanner.AbsoluteBookPath(book)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, book.FileName))
	http.ServeFile(w, r, path)
}

func (a *App) coverGet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil || book.Metadata.CoverPath == nil || *book.Metadata.CoverPath == "" {
		http.ServeFile(w, r, "web/static/img/placeholder-cover.svg")
		return
	}
	path := filepath.Join(a.Cfg.DataDir, filepath.FromSlash(*book.Metadata.CoverPath))
	if _, err := os.Stat(path); err != nil {
		http.ServeFile(w, r, "web/static/img/placeholder-cover.svg")
		return
	}
	http.ServeFile(w, r, path)
}

func (a *App) bookLookup(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	title := book.Metadata.DisplayTitle(book.FileName)
	author := ""
	if len(book.Authors) > 0 {
		author = book.Authors[0]
	}
	isbn := ""
	if book.Metadata.ISBN13 != nil {
		isbn = *book.Metadata.ISBN13
	}
	results, err := metadata.Lookup(r.Context(), title, author, isbn)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	components.LookupResults(id, results).Render(r.Context(), w)
}

func (a *App) bookApplyLookup(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanEditMetadata }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	var res metadata.LookupResult
	_ = json.Unmarshal([]byte(r.FormValue("payload")), &res)
	authors := res.Authors
	cats := res.Categories
	_ = a.Store.UpdateBookMetadata(r.Context(), id, res.Title, res.Subtitle, res.Description, res.Publisher, "", res.ISBN13, authors, cats)
	if res.CoverURL != "" {
		if data, ext, err := metadata.DownloadCover(r.Context(), res.CoverURL); err == nil {
			if rel, err := a.Scanner.SaveCoverBytes(id, data, ext); err == nil {
				_ = a.Store.SetBookCover(r.Context(), id, rel)
			}
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/books/%d", id), http.StatusSeeOther)
}

func (a *App) librariesList(w http.ResponseWriter, r *http.Request) {
	libs, _ := a.Store.ListLibraries(r.Context())
	a.render(w, r, "Libraries", "libraries", components.LibrariesPage(libs, a.Cfg.BooksDir, ""))
}

func (a *App) librariesCreate(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanManageLibrary }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	path := strings.TrimSpace(r.FormValue("path"))
	icon := r.FormValue("icon")
	if icon == "" {
		icon = "book"
	}
	watch := r.FormValue("watch") == "on"
	if name == "" || path == "" {
		libs, _ := a.Store.ListLibraries(r.Context())
		a.render(w, r, "Libraries", "libraries", components.LibrariesPage(libs, a.Cfg.BooksDir, "Name and path required."))
		return
	}
	// validate path under books dir or absolute existing
	check := path
	if !filepath.IsAbs(check) {
		check = filepath.Join(a.Cfg.BooksDir, path)
	}
	if err := os.MkdirAll(check, 0o755); err != nil {
		libs, _ := a.Store.ListLibraries(r.Context())
		a.render(w, r, "Libraries", "libraries", components.LibrariesPage(libs, a.Cfg.BooksDir, err.Error()))
		return
	}
	_, err := a.Store.CreateLibrary(r.Context(), name, icon, watch, []string{path})
	if err != nil {
		libs, _ := a.Store.ListLibraries(r.Context())
		a.render(w, r, "Libraries", "libraries", components.LibrariesPage(libs, a.Cfg.BooksDir, err.Error()))
		return
	}
	http.Redirect(w, r, "/libraries", http.StatusSeeOther)
}

func (a *App) librariesScan(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanManageLibrary }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	n, err := a.Scanner.ScanLibrary(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	log.Printf("scanned library %d: %d books", id, n)
	http.Redirect(w, r, "/libraries", http.StatusSeeOther)
}

func (a *App) librariesDelete(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanManageLibrary }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = a.Store.DeleteLibrary(r.Context(), id)
	http.Redirect(w, r, "/libraries", http.StatusSeeOther)
}

func (a *App) shelvesList(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	shelves, _ := a.Store.ListShelves(r.Context(), u.ID)
	a.render(w, r, "Shelves", "shelves", components.ShelvesPage(shelves, ""))
}

func (a *App) shelvesCreate(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		shelves, _ := a.Store.ListShelves(r.Context(), u.ID)
		a.render(w, r, "Shelves", "shelves", components.ShelvesPage(shelves, "Name required."))
		return
	}
	icon := r.FormValue("icon")
	if icon == "" {
		icon = "bookmark"
	}
	_, _ = a.Store.CreateShelf(r.Context(), u.ID, name, icon)
	http.Redirect(w, r, "/shelves", http.StatusSeeOther)
}

func (a *App) shelfDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	books, _ := a.Store.ShelfBooks(r.Context(), id)
	a.render(w, r, "Shelf", "shelves", components.ShelfDetailPage(id, books))
}

func (a *App) shelfDelete(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = a.Store.DeleteShelf(r.Context(), id, u.ID)
	http.Redirect(w, r, "/shelves", http.StatusSeeOther)
}

func (a *App) shelfAddBook(w http.ResponseWriter, r *http.Request) {
	shelfID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bookID, _ := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	_ = a.Store.AddToShelf(r.Context(), shelfID, bookID)
	http.Redirect(w, r, fmt.Sprintf("/books/%d", bookID), http.StatusSeeOther)
}

func (a *App) shelfRemoveBook(w http.ResponseWriter, r *http.Request) {
	shelfID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bookID, _ := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	_ = a.Store.RemoveFromShelf(r.Context(), shelfID, bookID)
	http.Redirect(w, r, fmt.Sprintf("/shelves/%d", shelfID), http.StatusSeeOther)
}

func (a *App) magicList(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	list, _ := a.Store.ListMagicShelves(r.Context(), u.ID)
	a.render(w, r, "Magic shelves", "magic", components.MagicShelvesPage(list, ""))
}

func (a *App) magicCreate(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	rules := models.MagicRules{
		Author:   strings.TrimSpace(r.FormValue("author")),
		Category: strings.TrimSpace(r.FormValue("category")),
		Series:   strings.TrimSpace(r.FormValue("series")),
		Format:   strings.TrimSpace(r.FormValue("format")),
		Status:   strings.TrimSpace(r.FormValue("status")),
		Query:    strings.TrimSpace(r.FormValue("query")),
	}
	if lib := r.FormValue("library_id"); lib != "" {
		rules.LibraryID, _ = strconv.ParseInt(lib, 10, 64)
	}
	if name == "" {
		list, _ := a.Store.ListMagicShelves(r.Context(), u.ID)
		a.render(w, r, "Magic shelves", "magic", components.MagicShelvesPage(list, "Name required."))
		return
	}
	ms, err := a.Store.CreateMagicShelf(r.Context(), u.ID, name, "sparkles", rules)
	if err != nil {
		list, _ := a.Store.ListMagicShelves(r.Context(), u.ID)
		a.render(w, r, "Magic shelves", "magic", components.MagicShelvesPage(list, err.Error()))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/magic-shelves/%d", ms.ID), http.StatusSeeOther)
}

func (a *App) magicDetail(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ms, err := a.Store.GetMagicShelf(r.Context(), id, u.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	books, _, _ := a.Store.ListBooks(r.Context(), store.BookFilter{
		Query: ms.Rules.Query, Author: ms.Rules.Author, Category: ms.Rules.Category,
		Series: ms.Rules.Series, Format: ms.Rules.Format, Status: ms.Rules.Status,
		LibraryID: ms.Rules.LibraryID, UserID: u.ID, Limit: 200,
	})
	a.render(w, r, ms.Name, "magic", components.MagicDetailPage(*ms, books))
}

func (a *App) magicDelete(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = a.Store.DeleteMagicShelf(r.Context(), id, u.ID)
	http.Redirect(w, r, "/magic-shelves", http.StatusSeeOther)
}

func (a *App) bookdropList(w http.ResponseWriter, r *http.Request) {
	files, _ := a.Store.ListBookdrop(r.Context())
	libs, _ := a.Store.ListLibraries(r.Context())
	a.render(w, r, "BookDrop", "bookdrop", components.BookdropPage(files, libs))
}

func (a *App) bookdropImport(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanUpload || p.CanManageLibrary }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	libID, _ := strconv.ParseInt(r.FormValue("library_id"), 10, 64)
	bookID, err := a.Bookdrop.Import(r.Context(), id, libID, a.Cfg.BooksDir, a.Scanner)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/books/%d", bookID), http.StatusSeeOther)
}

func (a *App) bookdropReject(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if f, err := a.Store.GetBookdrop(r.Context(), id); err == nil {
		_ = os.Remove(f.Path)
	}
	_ = a.Store.DeleteBookdrop(r.Context(), id)
	http.Redirect(w, r, "/bookdrop", http.StatusSeeOther)
}

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if err := auth.RequirePerm(u, func(p models.Permissions) bool { return p.CanUpload }); err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	libID, _ := strconv.ParseInt(r.FormValue("library_id"), 10, 64)
	lib, err := a.Store.GetLibrary(r.Context(), libID)
	if err != nil || len(lib.Paths) == 0 {
		http.Error(w, "invalid library", 400)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer file.Close()
	format, ok := metadata.FormatFromPath(hdr.Filename)
	if !ok {
		http.Error(w, "unsupported format", 400)
		return
	}
	root := lib.Paths[0].Path
	if !filepath.IsAbs(root) {
		root = filepath.Join(a.Cfg.BooksDir, root)
	}
	_ = os.MkdirAll(root, 0o755)
	dest := filepath.Join(root, filepath.Base(hdr.Filename))
	out, err := os.Create(dest)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, err := io.Copy(out, file)
	out.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	meta, _ := metadata.Extract(dest)
	// bounded so a slow provider can't stall the upload response
	ectx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	metadata.Enrich(ectx, &meta)
	cancel()
	hash, _ := metadata.HashFile(dest)
	lpID := lib.Paths[0].ID
	book := models.Book{
		LibraryID: libID, LibraryPathID: &lpID, FileName: filepath.Base(hdr.Filename),
		Format: format, FileSize: n,
	}
	if hash != "" {
		book.FileHash = &hash
	}
	id, err := a.Store.UpsertBook(r.Context(), book, meta)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(meta.CoverData) > 0 {
		if rel, err := a.Scanner.SaveCoverBytes(id, meta.CoverData, meta.CoverExt); err == nil {
			_ = a.Store.SetBookCover(r.Context(), id, rel)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/books/%d", id), http.StatusSeeOther)
}

func (a *App) readBook(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := a.currentUser(r)
	prog, _ := a.Store.GetProgress(r.Context(), u.ID, id)
	a.render(w, r, "Reading · "+book.Metadata.DisplayTitle(book.FileName), "books", components.ReaderPage(*book, prog))
}

func (a *App) streamFile(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := a.Scanner.AbsoluteBookPath(book)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.ServeFile(w, r, path)
}

func (a *App) cbzPage(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	n, _ := strconv.Atoi(chi.URLParam(r, "n"))
	book, err := a.Store.GetBook(r.Context(), id)
	if err != nil || book.Format != "cbz" {
		http.NotFound(w, r)
		return
	}
	path, err := a.Scanner.AbsoluteBookPath(book)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	pages, err := metadata.ListCBZPages(path)
	if err != nil || n < 0 || n >= len(pages) {
		http.NotFound(w, r)
		return
	}
	data, err := metadata.ReadZipEntry(path, pages[n])
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	ext := strings.ToLower(filepath.Ext(pages[n]))
	ct := "image/jpeg"
	switch ext {
	case ".png":
		ct = "image/png"
	case ".webp":
		ct = "image/webp"
	case ".gif":
		ct = "image/gif"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(data)
}

func (a *App) saveProgress(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body struct {
		Percent  float64         `json:"percent"`
		Position json.RawMessage `json:"position"`
		Status   string          `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.Store.SaveProgress(r.Context(), u.ID, id, body.Percent, body.Position, body.Status); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) getProgress(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := a.Store.GetProgress(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	users, _ := a.Store.ListUsers(r.Context())
	opds, _ := a.Store.ListOPDSUsers(r.Context())
	libs, _ := a.Store.ListLibraries(r.Context())
	a.render(w, r, "Settings", "settings", components.SettingsPage(u, users, opds, libs, theme.Names, a.themeFor(r), ""))
}

func (a *App) setTheme(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	t := theme.Normalize(r.FormValue("theme"))
	if u := a.currentUser(r); u != nil {
		_ = a.Store.UpdateUserTheme(r.Context(), u.ID, t)
	}
	http.SetCookie(w, &http.Cookie{Name: "theme", Value: t, Path: "/", MaxAge: 365 * 24 * 3600})
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}
	_ = r.ParseForm()
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	display := strings.TrimSpace(r.FormValue("display_name"))
	hash, err := auth.HashPassword(password)
	if err != nil || username == "" || len(password) < 8 {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	perms := models.Permissions{
		CanUpload: r.FormValue("can_upload") == "on",
		CanDownload: r.FormValue("can_download") == "on" || r.FormValue("can_download") == "",
		CanEditMetadata: r.FormValue("can_edit_metadata") == "on",
		CanManageLibrary: r.FormValue("can_manage_library") == "on",
	}
	admin := r.FormValue("is_admin") == "on"
	if display == "" {
		display = username
	}
	_, _ = a.Store.CreateUser(r.Context(), username, hash, display, admin, perms)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (a *App) updateUserPerms(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	perms := models.Permissions{
		CanUpload: r.FormValue("can_upload") == "on",
		CanDownload: r.FormValue("can_download") == "on",
		CanEditMetadata: r.FormValue("can_edit_metadata") == "on",
		CanManageLibrary: r.FormValue("can_manage_library") == "on",
	}
	admin := r.FormValue("is_admin") == "on"
	_ = a.Store.UpdateUserPermissions(r.Context(), id, perms, admin)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (a *App) createOPDSUser(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}
	_ = r.ParseForm()
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	hash, err := auth.HashPassword(password)
	if err != nil || username == "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	uid := u.ID
	_ = a.Store.CreateOPDSUser(r.Context(), username, hash, &uid)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (a *App) deleteOPDSUser(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = a.Store.DeleteOPDSUser(r.Context(), id)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (a *App) opdsBasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="e-biblioteca OPDS"`)
			http.Error(w, "auth required", 401)
			return
		}
		ou, err := a.Store.GetOPDSUser(r.Context(), user)
		if err != nil || !auth.CheckPassword(ou.PasswordHash, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="e-biblioteca OPDS"`)
			http.Error(w, "unauthorized", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) opdsRoot(w http.ResponseWriter, r *http.Request) {
	base := schemeHost(r)
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=navigation")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog">
  <id>%s/opds</id>
  <title>e-biblioteca</title>
  <updated>%s</updated>
  <link rel="self" href="%s/opds" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>
  <link rel="start" href="%s/opds" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>
  <entry>
    <title>All books</title>
    <id>%s/opds/catalog</id>
    <updated>%s</updated>
    <link rel="subsection" href="%s/opds/catalog" type="application/atom+xml;profile=opds-catalog;kind=acquisition"/>
    <content type="text">Browse the full library</content>
  </entry>
</feed>`, base, nowAtom(), base, base, base, nowAtom(), base)
}

func (a *App) opdsCatalog(w http.ResponseWriter, r *http.Request) {
	books, _, err := a.Store.ListBooks(r.Context(), store.BookFilter{Limit: 200})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	base := schemeHost(r)
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog">`)
	fmt.Fprintf(&b, `<id>%s/opds/catalog</id><title>All books</title><updated>%s</updated>`, base, nowAtom())
	fmt.Fprintf(&b, `<link rel="self" href="%s/opds/catalog" type="application/atom+xml;profile=opds-catalog;kind=acquisition"/>`, base)
	for _, book := range books {
		title := book.Metadata.DisplayTitle(book.FileName)
		mime := mimeFor(book.Format)
		fmt.Fprintf(&b, `<entry><title>%s</title><id>%s/books/%d</id><updated>%s</updated>`,
			xmlEscape(title), base, book.ID, nowAtom())
		if len(book.Authors) > 0 {
			fmt.Fprintf(&b, `<author><name>%s</name></author>`, xmlEscape(book.Authors[0]))
		}
		fmt.Fprintf(&b, `<link rel="http://opds-spec.org/acquisition" href="%s/opds/download/%d" type="%s"/>`, base, book.ID, mime)
		fmt.Fprintf(&b, `<link rel="http://opds-spec.org/image" href="%s/covers/%d" type="image/jpeg"/>`, base, book.ID)
		b.WriteString(`</entry>`)
	}
	b.WriteString(`</feed>`)
	_, _ = w.Write([]byte(b.String()))
}

func schemeHost(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	}
	return scheme + "://" + r.Host
}

func nowAtom() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func mimeFor(format string) string {
	switch format {
	case "epub":
		return "application/epub+zip"
	case "pdf":
		return "application/pdf"
	case "cbz":
		return "application/vnd.comicbook+zip"
	case "mp3":
		return "audio/mpeg"
	case "m4b", "m4a":
		return "audio/mp4"
	case "opus":
		return "audio/opus"
	default:
		return "application/octet-stream"
	}
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

