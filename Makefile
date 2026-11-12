.PHONY: up down ingest test coverage cover

# Покрытие всего бэкенда: итоговая строка total.
cover:
	cd backend && go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1
