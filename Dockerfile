# Build
FROM golang:1.24-bookworm AS build
WORKDIR /src
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/e-biblioteca ./cmd/server

# Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/e-biblioteca /app/e-biblioteca
COPY web /app/web
ENV HTTP_PORT=8080 \
    DATA_DIR=/data \
    BOOKS_DIR=/books \
    BOOKDROP_DIR=/bookdrop
EXPOSE 8080
VOLUME ["/data", "/books", "/bookdrop"]
ENTRYPOINT ["/app/e-biblioteca"]
