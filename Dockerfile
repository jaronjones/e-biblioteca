# Build
# Prefer images commonly present locally. Host Docker Hub pulls may fail when
# IPv6 is broken (dial tcp [2600:...]:443: connect: invalid argument).
# GOTOOLCHAIN=auto downloads the go.mod toolchain if the image Go is newer/older.
FROM golang:1.26-bookworm AS build
WORKDIR /src
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/e-biblioteca ./cmd/server

# Runtime: alpine is widely cached; static binary needs no glibc.
# ca-certificates for outbound HTTPS (metadata providers); wget for healthcheck.
FROM alpine:3
RUN apk add --no-cache ca-certificates wget
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
