.PHONY: generate build run tidy docker-up docker-down

export PATH := $(shell go env GOPATH)/bin:$(PATH)

generate:
	templ generate

tidy:
	go mod tidy

build: generate tidy
	go build -o bin/e-biblioteca ./cmd/server

run: generate
	go run ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
