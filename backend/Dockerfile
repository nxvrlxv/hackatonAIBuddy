# multi-stage сборка всех бинарей из cmd/
# backend/Dockerfile

# ---------- стадия сборки ----------
FROM golang:1.27.1-alpine AS builder

WORKDIR /src

# Сначала только файлы зависимостей: этот слой кэшируется, пока они не меняются
COPY go.mod go.sum ./
RUN go mod download

# Затем весь код (миграции встроятся в бинарь через go:embed)
COPY . .

# Статический бинарь под Linux
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -ldflags="-s -w" -o /out/migrate ./cmd/migrator

# ---------- стадия запуска ----------
FROM alpine:3.20

# Сертификаты и непривилегированный пользователь без пароля и домашней папки
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -s /sbin/nologin app

WORKDIR /app
COPY --from=builder /out/ /app/

USER app

CMD ["/app/migrate"]