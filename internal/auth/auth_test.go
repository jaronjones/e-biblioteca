package auth

import (
	"testing"

	"github.com/jjones/e-biblioteca/internal/models"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct-horse-battery") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password should not match")
	}
}

func TestRequirePerm(t *testing.T) {
	if err := RequirePerm(nil, func(p models.Permissions) bool { return true }); err != ErrUnauthorized {
		t.Fatalf("nil user: got %v", err)
	}
	admin := &models.User{IsAdmin: true}
	if err := RequirePerm(admin, func(p models.Permissions) bool { return false }); err != nil {
		t.Fatalf("admin bypasses check: got %v", err)
	}
	user := &models.User{Permissions: models.Permissions{CanDownload: true}}
	if err := RequirePerm(user, func(p models.Permissions) bool { return p.CanDownload }); err != nil {
		t.Fatalf("permitted: got %v", err)
	}
	if err := RequirePerm(user, func(p models.Permissions) bool { return p.CanUpload }); err != ErrForbidden {
		t.Fatalf("denied: got %v", err)
	}
}
