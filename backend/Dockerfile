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
# Каждый бинарь кладём в /out под коротким именем (migrate — исторически так в compose).
# mcp-corporate пока не собираем: main.go пустой, сервис появится на этапе 7.
RUN go build -ldflags="-s -w" -o /out/migrate ./cmd/migrator \
    && go build -ldflags="-s -w" -o /out/ingest ./cmd/ingest \
    && go build -ldflags="-s -w" -o /out/api ./cmd/api \
    && go build -ldflags="-s -w" -o /out/mcp-buddy ./cmd/mcp-buddy

# ---------- стадия запуска ----------
FROM alpine:3.20

# Сертификаты и непривилегированный пользователь без пароля и домашней папки
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -s /sbin/nologin app

WORKDIR /app
COPY --from=builder /out/ /app/

USER app

# Команда по умолчанию — миграции; остальные сервисы задают свою command в compose.
CMD ["/app/migrate"]