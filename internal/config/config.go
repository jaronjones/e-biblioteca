package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort      string
	DatabaseURL   string
	DataDir       string
	BooksDir      string
	BookdropDir   string
	SecureCookies bool
}

func Load() Config {
	return Config{
		HTTPPort:      getenv("HTTP_PORT", "8080"),
		DatabaseURL:   getenv("DATABASE_URL", "postgres://ebiblioteca:ebiblioteca@localhost:5432/ebiblioteca?sslmode=disable"),
		DataDir:       getenv("DATA_DIR", "./data"),
		BooksDir:      getenv("BOOKS_DIR", "./books"),
		BookdropDir:   getenv("BOOKDROP_DIR", "./bookdrop"),
		SecureCookies: getenvBool("SECURE_COOKIES", false),
	}
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
