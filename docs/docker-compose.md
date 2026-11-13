# Docker Compose и Ollama

## Сервисы

| Сервис | Назначение |
|---|---|
| postgres | PostgreSQL с pgvector; данные в pg_data |
| migrator | Применяет миграции и завершает работу |
| ollama | HTTP-сервер диалоговой и embedding-моделей; файлы моделей в ollama_data |
| ollama-models | Ждёт готовности Ollama, скачивает обе модели и завершается |
| api | Публичный Go API, авторизация, хранение и SSE-прокси |
| mcp-buddy | whoami и search_docs; для поиска обращается в Ollama |
| agent | Flask/LangGraph, обращается в Go, MCP и Ollama |
| ingest | Отдельный запуск индексации корпоративных документов |

Загрузка ollama-models:
1. qwen3-embedding:0.6b — EMBEDDED_MODEL, используется Go.
2. qwen3:4b-instruct — OLLAMA_MODEL, используется Python. LLM_MODEL Go задан тем же именем.

agent, mcp-buddy и ingest ждут успешного завершения ollama-models.
Healthcheck Ollama проверяет доступность сервера; наличие моделей обеспечивает
отдельный загрузчик. Ошибка скачивания останавливает запуск зависимых сервисов.

Модели хранятся в Docker volume ollama_data, подключённом к /root/.ollama.
Пересборка Python/Go и обычный docker compose down модели не удаляют.
docker compose down -v удаляет volumes, включая модели И PostgreSQL: не используйте
эту команду для обычного перезапуска.

## Настройки и адреса

Корневой .env создаётся по .env.example. Если файл уже есть, дополните его,
не перезаписывая секреты. Для моделей используются:

```dotenv
OLLAMA_DOCKER_URL=http://ollama:11434
OLLAMA_URL=http://localhost:11434
OLLAMA_PORT=11434
OLLAMA_IMAGE=ollama/ollama:latest
EMBEDDED_MODEL=qwen3-embedding:0.6b
OLLAMA_MODEL=qwen3:4b-instruct
LLM_MODEL=qwen3:4b-instruct
INDEX_VERSION=qwen3-embedding-06b-v1
```

OLLAMA_IMAGE можно закрепить на проверенном теге или digest для повторяемой сборки.

| Где выполняется запрос | URL | Как задаётся |
|---|---|---|
| Python внутри Compose | http://ollama:11434 | OLLAMA_DOCKER_URL подставляется в OLLAMA_URL контейнера |
| Go MCP / ingest внутри Compose | http://ollama:11434 | Такая же подстановка в OLLAMA_URL |
| ollama-models | http://ollama:11434 | OLLAMA_DOCKER_URL подставляется в клиентский OLLAMA_HOST |
| Python/Go локально на компьютере | http://localhost:11434 | OLLAMA_URL в локальном окружении |
| curl/Postman на компьютере | http://localhost:11434 | Опубликованный порт OLLAMA_PORT |
| Фронтенд | http://localhost:8080 | Обращается к Go, адрес Ollama ему не нужен |

Внутри контейнера localhost — сам контейнер. Поэтому http://localhost:11434
нельзя использовать из контейнеров agent/api/mcp-buddy.
Имя ollama разрешается только в сети Compose.
В конфигурации сервера OLLAMA_HOST=0.0.0.0:11434 — адрес прослушивания,
а не URL, который нужно передать фронтенду.

Порт Ollama опубликован только на 127.0.0.1. Если 11434 занят установленной
на компьютере Ollama, задайте OLLAMA_PORT=11435. Тогда локальный OLLAMA_URL
будет http://localhost:11435, а контейнеры продолжат использовать http://ollama:11434.
Из WSL доступ к опубликованному порту зависит от режима сети Docker Desktop/WSL;
внутри Compose адрес по имени сервиса от этого не зависит.

## Запуск

Из корня:
```bash
docker compose up --build -d api mcp-buddy agent
docker compose logs -f ollama-models
docker compose exec ollama ollama list
```

Ollama и загрузчик стартуют автоматически по зависимостям.
Первый запуск скачивает образ и несколько гигабайт моделей; последующие используют volume.
Загрузчик при повторном запуске проверяет модели и докачивает отсутствующие слои.

Только Ollama и модели, без БД/Go:
```bash
docker compose up -d ollama-models
docker compose logs -f ollama-models
```

После изменения имени модели повторно запустите загрузчик и пересоздайте потребителей:
```bash
docker compose up -d --force-recreate ollama-models
docker compose up -d agent mcp-buddy
```

## GPU

Основной Compose работает на CPU и не требует NVIDIA.
Для NVIDIA доступен дополнительный файл:
```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up --build -d api mcp-buddy agent
docker compose exec ollama ollama ps
```

GPU-файл добавляет gpus: all только сервису ollama.
Для Docker Desktop на Windows необходимы NVIDIA, совместимые драйверы и WSL2.
Само наличие GPU-файла не подтверждает доступность видеокарты; ollama ps после
запроса показывает, где выполняется модель.
См. [Docker GPU](https://docs.docker.com/desktop/features/gpu/)
и [Ollama Docker](https://docs.ollama.com/docker).

## Проверка API и индексация

```bash
curl http://localhost:11434/api/tags

curl http://localhost:11434/api/embed -d '{
  "model":"qwen3-embedding:0.6b",
  "input":"Проверка доступа к Git"
}'

curl http://localhost:11434/api/chat -d '{
  "model":"qwen3:4b-instruct",
  "messages":[{"role":"user","content":"Ответь одним словом: привет"}],
  "stream":false
}'
```

qwen3-embedding:0.6b имеет стандартную размерность 1024, совместимую с
buddy.chunks.embedding vector(1024). [Карточка модели](https://huggingface.co/Qwen/Qwen3-Embedding-0.6B).
SQL-схему менять не нужно.

При переходе с другой embedding-модели старые векторы нужно пересчитать:
измените INDEX_VERSION и выполните:
```bash
docker compose run --rm ingest
```
В примере версия уже изменена на qwen3-embedding-06b-v1.
Загрузка моделей сама по себе не запускает переиндексацию и не меняет данные БД.
Поиск по истории чатов остаётся отключённым до реализации соответствующих Go API.

## Внешний сервер Ollama

Можно задать OLLAMA_DOCKER_URL адресом внешнего сервера, доступным контейнерам.
При этом текущая цепочка зависимостей всё ещё поднимает локальный ollama:
чтобы полностью исключить его, нужен отдельный Compose override зависимостей.
Для обычного локального запуска оставьте http://ollama:11434.

## Проверочный интерфейс и память

HTML-чат: http://localhost:8001/demo. Адрес Go для браузера задаёт PUBLIC_GO_API_URL.
[Инструкция проверки RAG](rag-testing.md).

Для локального запуска Ollama ограничена одной загруженной моделью и одним
параллельным запросом (OLLAMA_MAX_LOADED_MODELS, OLLAMA_NUM_PARALLEL).
OLLAMA_CONTEXT_LENGTH задаёт её стандартный контекст; Python отдельно передаёт
CONTEXT_TOKENS. Значения из agent/.env имеют приоритет над корневым .env.
