# План бэкенда AI-buddy

Сегодня 13.11, код замораживаем вечером 15.11, загрузить нужно 16.11 до 12:00 (жёсткий срок 14:00 МСК).

## Схема, к которой идём

```
фронт ──Bearer──▶ api (Go) ──X-Agent-Key + Bearer──▶ agent (Python)
                   │  ▲                                   │
                   │  └──── /internal/agent/* (история) ◀─┤
                   ▼                                      ▼
               Postgres ◀──────────────── mcp-buddy (Go), mcp-corporate (Go)
```

---

## Этап 0. Договорённости (13.11, до кода)

- [ ] Отправить товарищу ответ с решениями по контракту (см. раздел «Договорённости с агентом» в конце).
- [ ] Получить ветку агента: код, сервис в compose, точные JSON-тела запросов.
- [ ] Согласовать статусы хода: `running | completed | failed | interrupted`.
- [ ] Решить в команде, кто делает чек-лист и `mcp-corporate` (этап 7). Если я, они уходят на 15.11 и урезаются.

---

## Этап 1. Доделать REST и авторизацию (13.11, примерно 2 часа)

1. **`auth/context.go`**: сейчас паника на пустом контексте.
   ```go
   claims, ok := ctx.Value(contKey{}).(*Claims)
   if !ok || claims == nil {
       return nil, ErrUnauthenticated
   }
   ```
2. **`server.go`**:
   - переименовать `TaskManager`/`tasks` → `TokenManager`/`tokens`;
   - добавить `New`, `Handler()`, `cors`.
3. **`middleware.go`**: вместо `err.Error()` отдавать `"invalid token"`.
4. **`cmd/api/main.go`**: конфиг → `NewManager` → пул → репозиторий → `http.Server` с корректной остановкой.
5. **Тесты**: `handlers_test.go` + `TestClaimsContext`.
6. **`users.go`**: исправить текст ошибки в `GetUserByID` (сейчас там «get user by login»).

**Готово, когда** `curl POST /api/login` возвращает токен, `GET /api/me` с ним отдаёт профиль, а без токена — 401 JSON.

## Этап 2. `whoami` и ключ агента (13.11, примерно 1,5 часа)

1. **`whoami`**:
   - добавить в ответ `id` и `team_id`;
   - принимать интерфейс `UserGetter` вместо `*buddy.Repository`;
   - тест с фейком.
2. **Конфиг**: `AGENT_KEY` и `AGENT_URL` (`http://agent:8001`).
3. **`requireAgentKey`**: сравнение через `subtle.ConstantTimeCompare`, при ошибке 401.
4. **Маршруты `/internal/agent/*`**: `requireAgentKey(requireAuth(h))`.
5. **Тесты**: без ключа, неверный ключ, ключ без токена, всё верно.

## Этап 3. Миграция `0004_chat` (13.11, примерно 1,5 часа)

```sql
CREATE TABLE buddy.conversations (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES buddy.users(id) ON DELETE CASCADE,
    title         TEXT   NOT NULL DEFAULT '',
    last_sequence BIGINT NOT NULL DEFAULT 0,
    summary_text             TEXT   NOT NULL DEFAULT '',
    summary_through_sequence BIGINT NOT NULL DEFAULT 0,
    summary_version          INT    NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_conversations_user ON buddy.conversations (user_id, updated_at DESC);

CREATE TABLE buddy.runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   BIGINT NOT NULL REFERENCES buddy.conversations(id) ON DELETE CASCADE,
    client_message_id TEXT   NOT NULL,
    status            TEXT   NOT NULL DEFAULT 'running'
        CHECK (status IN ('running','completed','failed','interrupted')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ,
    UNIQUE (conversation_id, client_message_id)
);
CREATE UNIQUE INDEX runs_one_running ON buddy.runs (conversation_id) WHERE status = 'running';

CREATE TABLE buddy.messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES buddy.conversations(id) ON DELETE CASCADE,
    run_id          UUID   NOT NULL REFERENCES buddy.runs(id) ON DELETE CASCADE,
    sequence_number BIGINT NOT NULL,
    event_key       TEXT   NOT NULL,
    role            TEXT   NOT NULL CHECK (role IN ('user','assistant','tool','system')),
    payload         JSONB  NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, sequence_number),
    UNIQUE (run_id, event_key)
);
```

- [ ] `down`: удалить таблицы в обратном порядке.
- [ ] `migrations_test.go`: версия 4, после отката 3.
- [ ] В сид добавить стажёра в `adverts` или `payments`.

---

## Этап 4. Хранилище чатов (14.11, до обеда, самый большой этап)

**Слои:**
- `postgres/buddy/chat.go` — SQL;
- `internal/chat/service.go` — правила и ошибки;
- `httpapi/agent_handlers.go` — HTTP.

**Ошибки сервиса и их коды:**

| Ошибка сервиса | HTTP |
|---|---|
| `ErrNotFound` (нет чата или чужой) | 404 |
| `ErrDuplicateMessage` (`client_message_id` уже был) | 409 |
| `ErrRunInProgress` (уже есть `running`) | 409 |
| `ErrRunNotActive` (запись в завершённый ход) | 409 |

**Внутренние ручки** (`/internal/agent`, ключ + токен):

| Метод и путь | Что делает |
|---|---|
| `POST /conversations` | создать чат |
| `GET /conversations` | список чатов пользователя |
| `GET /conversations/{id}/messages?limit&before` | полный `payload` |
| `POST /conversations/{id}/turns` | начать ход, вернуть `run_id`, историю и пустое резюме |
| `POST /conversations/{id}/runs/{run_id}/messages` | добавить сообщение |
| `POST /conversations/{id}/runs/{run_id}/finish` | завершить ход |

**Транзакция `turns`:**
1. `SELECT ... FROM conversations WHERE id=$1 AND user_id=$2 FOR UPDATE`. Нет строки → 404.
2. `UPDATE runs SET status='interrupted', finished_at=now() WHERE conversation_id=$1 AND status='running' AND last_activity_at < now() - interval '5 minutes'`.
3. `INSERT runs`. Нарушение уникальности: по `client_message_id` → `ErrDuplicateMessage`, по `runs_one_running` → `ErrRunInProgress`. Различать по `pgErr.ConstraintName`.
4. `last_sequence + 1`, затем `INSERT` user-сообщения.
5. Загрузить историю (последние N сообщений, сортировка по `sequence_number`), `COMMIT`.

В ответе `summary` всегда `{"text":"","version":0,"through_sequence":0}` (пока `SUMMARY_ENABLED=false`).

**Транзакция `append`:**
1. Есть запись с таким `(run_id, event_key)` → вернуть её. Это повтор, принимается даже после finish.
2. `SELECT status FROM runs ... JOIN conversations ... user_id ... FOR UPDATE`. Не `running` → `ErrRunNotActive`.
3. `UPDATE conversations SET last_sequence = last_sequence + 1 ... RETURNING last_sequence`.
4. `INSERT message`, обновить `runs.last_activity_at = now()`.

**`finish`:**
```sql
UPDATE buddy.runs SET status = $2, finished_at = now()
WHERE id = $1 AND status = 'running'
```
Если 0 строк, вернуть 200 с текущим статусом, ничего не меняя.

**Тесты:**
- Сервис: фейковый репозиторий, проверка преобразования ошибок.
- Хендлеры: `httptest` с фейковым сервисом, все коды из таблицы.
- Репозиторий: pgxmock (для этого `Repository.pool` → интерфейс `DB`).
- Интеграционный тест на реальном Postgres, пропускается, если `DATABASE_URL` не задан:
  - повторная отправка → 409;
  - два параллельных `turns` → один 409;
  - чужой `user_id` → 404;
  - двойной `finish`;
  - запись после `interrupted`.

## Этап 5. Публичные ручки и прокси (14.11, после обеда)

| Метод и путь | Что делает |
|---|---|
| `POST /api/conversations` | создать чат |
| `GET /api/conversations` | список чатов |
| `GET /api/conversations/{id}/messages` | только `user` и `assistant` с непустым `content` |
| `POST /api/chat` | SSE-прокси на `agent` |

**Прокси:** `httputil.ReverseProxy`, `FlushInterval: -1`. В `Rewrite`: `Del("X-Agent-Key")`, затем `Set` своего ключа, `Authorization` пробросить как есть. Обернуть в `requireAuth`. У `http.Server` не ставить `WriteTimeout`, иначе оборвётся длинный поток.

**Тест:** фейковый агент на `httptest.Server` шлёт два SSE-события с паузой. Проверить, что первое доходит до второго, ключ подменён, ключ клиента до агента не дошёл.

## Этап 6. Compose и сквозная проверка (14.11, вечер)

- [ ] Dockerfile собирает бинарники `api`, `mcp-buddy`, `mcp-corporate`, `migrate`, `ingest`.
- [ ] Сервисы и порты: `postgres`, `migrator`, `ingest` (одноразовый), `api:8080`, `mcp-buddy:8090`, `mcp-corporate:8091`, `agent:8001`, `frontend`.
- [ ] Обращения по именам сервисов: `AGENT_URL=http://agent:8001`, `MCP_BUDDY_URL=http://mcp-buddy:8090/mcp`.
- [ ] `.env.example` со всеми переменными без секретов.
- [ ] Сквозной прогон: `docker compose down -v && docker compose up --build` → вход → создать чат → вопрос по документам → ответ стримится → история сохранилась.

---

## Этап 7. Сценарии кейса (15.11, или параллельно у другого человека)

1. **Чек-лист онбординга** (`mcp-buddy`):
   - таблицы `tasks` и `user_tasks` в `0005_checklist`, сид на 5–7 задач;
   - инструменты `get_checklist` и `complete_task` (пользователь из токена);
   - REST `GET /api/checklist`, `PATCH /api/checklist/{id}`.
2. **`mcp-corporate`** (имитация, данные в памяти или в сиде):
   - `get_absences` / `request_absence`;
   - `get_calendar` / `book_meeting`;
   - та же проверка Bearer через `mcpserver.Verifier`.
3. **Журнал действий** (если останется время): таблица `action_log`, запись при `complete_task`, `request_absence`, `book_meeting`.

**Порядок урезания при нехватке времени:** журнал → календарь → отгулы. Чат, прокси и чек-лист не урезаем.

## Этап 8. Финиш (15.11)

- [ ] Мелкая чистка:
  - удалить два старых теста в `config` (проверяют загрузку `.env`, которую сознательно убрали);
  - удалить `other_migrations/`;
  - исправить опечатку «внутреннней» в `search_docs`;
  - убрать устаревшие комментарии;
  - перенести `NewManager` в `mcp-buddy/main.go` сразу после `config.Load`.
- [ ] Покрытие: `make cover` → `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out`. Цель больше 80%. `cmd/*` вынести из подсчёта или держать `main` тонким.
- [ ] README: схема, запуск одной командой, тестовые логины, переменные окружения, команда для покрытия.
- [ ] **Вечером 15.11 — заморозка кода.**

## 16.11, утро

- [ ] Свежий клон, `docker compose up --build` с нуля.
- [ ] Прогон всех четырёх демо-сценариев.
- [ ] Загрузить решение до 12:00 (жёсткий срок 14:00).

---

## Сводка по дням

| День | Что делаем | Что получаем |
|---|---|---|
| **13.11** | этапы 0–3 | REST с входом, `whoami`, ключ агента, миграция 0004 |
| **14.11** | этапы 4–6 | хранилище чатов, прокси, работающий сквозной чат в compose |
| **15.11** | этапы 7–8 | сценарии, покрытие больше 80%, README, заморозка |
| **16.11** | проверка и загрузка | решение загружено до 12:00 |

**Главный риск** — ветка агента приходит поздно. Тогда этап 5 проверяем на фейковом агенте в тестах, а сквозной прогон (этап 6) переносится на утро 15.11.

---

## Договорённости с агентом

1. `whoami` без аргумента `token`, только заголовок `Authorization`.
2. Одна миграция `0004_chat` в `backend/migrations`; `0004_agent_memory` агента удаляется, 0005 для чатов нет.
3. Пути: `/internal/agent/conversations/{id}/runs/{run_id}/messages` и `.../finish`.
4. Фронт создаёт чат через `POST /api/conversations`, `/api/chat` только проксирует.
5. `HISTORY_SEARCH_ENABLED=false`: поиск и индексация истории (§3–4) отложены.
6. `SUMMARY_ENABLED=false`: `turns` отдаёт пустое резюме, `PUT /summary` (§5) пока нет; колонки в `conversations` уже заложены.
7. Повторный `finish` возвращает 200 с текущим статусом и ничего не меняет.
8. Протухание через 5 минут без активности: каждый append продлевает. Запись в неактивный ход → 409 `run not active`. Повтор с тем же `event_key` возвращает существующую запись даже после finish. Таймаут на вызов модели в агенте меньше 5 минут.
9. Для агента — полный `payload` с вызовами инструментов; для фронта — только видимый текст.
