package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/jjones/e-biblioteca/internal/models"
)

const (
	sessionUserKey = "user_id"
	ctxUserKey     = ctxKey("user")
	ctxCSRFKey     = ctxKey("csrf")
)

type ctxKey string

type Manager struct {
	Sessions *scs.SessionManager
}

func NewManager(pool *pgxpool.Pool, secure bool) *Manager {
	sm := scs.New()
	sm.Store = pgxstore.NewWithCleanupInterval(pool, 30*time.Minute)
	sm.Lifetime = 7 * 24 * time.Hour
	sm.Cookie.Name = "ebiblioteca_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = secure
	sm.Cookie.Path = "/"
	return &Manager{Sessions: sm}
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (m *Manager) Login(ctx context.Context, _ *http.Request, _ http.ResponseWriter, userID int64) {
	// Renew before writing session data after privilege change (scs docs).
	_ = m.Sessions.RenewToken(ctx)
	m.Sessions.Put(ctx, sessionUserKey, userID)
}

// CSRFToken returns the session CSRF token, creating one if needed.
func (m *Manager) CSRFToken(ctx context.Context) string {
	if v, ok := m.Sessions.Get(ctx, "csrf_token").(string); ok && v != "" {
		return v
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to time-based entropy.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	tok := hex.EncodeToString(b[:])
	m.Sessions.Put(ctx, "csrf_token", tok)
	return tok
}

func (m *Manager) ValidCSRF(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	expected, _ := m.Sessions.Get(ctx, "csrf_token").(string)
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

func (m *Manager) Logout(ctx context.Context) {
	m.Sessions.Destroy(ctx)
}

func (m *Manager) UserID(ctx context.Context) (int64, bool) {
	v := m.Sessions.Get(ctx, sessionUserKey)
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	case string:
		id, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return id, true
	default:
		return 0, false
	}
}

func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, ctxUserKey, u)
}

func UserFromContext(ctx context.Context) (*models.User, bool) {
	u, ok := ctx.Value(ctxUserKey).(*models.User)
	return u, ok && u != nil
}

func WithCSRF(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxCSRFKey, token)
}

func CSRFFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxCSRFKey).(string)
	return v
}

var ErrUnauthorized = errors.New("unauthorized")
var ErrForbidden = errors.New("forbidden")

func RequirePerm(u *models.User, check func(models.Permissions) bool) error {
	if u == nil {
		return ErrUnauthorized
	}
	if u.IsAdmin {
		return nil
	}
	if !check(u.Permissions) {
		return ErrForbidden
	}
	return nil
}
