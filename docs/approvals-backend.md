# Апрув задач наставником — что сделать на бэкенде

Контракт (ручки, JSON, статусы): [approvals-contract.md](approvals-contract.md). Здесь — как это лечь на наш код.
Фронт и агент делаются под этот контракт параллельно, поэтому формат ответов не менять без договорённости.

Оценка: 2–3 часа вместе с тестами. Порядок шагов ниже — от базы к ручкам, каждый шаг проверяется отдельно.

---

## Шаг 1. Миграция `0008_task_review`

Файлы: `backend/migrations/0008_task_review.up.sql` / `.down.sql`.

**up:**
- В `buddy.intern_tasks` CHECK статуса сейчас `('todo','in_progress','done')` — расширить до `('todo','in_progress','review','done')`.
  Ограничение безымянное: найти его имя (`\d buddy.intern_tasks`, обычно `intern_tasks_status_check`), `DROP CONSTRAINT` + `ADD CONSTRAINT ... CHECK (...)`.
- Связка `CHECK ((status = 'done') = (completed_at IS NOT NULL))` остаётся как есть — `review` не трогает `completed_at`.
- Новые колонки:
  - `submitted_at  TIMESTAMPTZ NULL` — когда отправлена на проверку;
  - `reviewed_by   BIGINT NULL REFERENCES buddy.users(id)` — кто принял/отклонил последним;
  - `reviewed_at   TIMESTAMPTZ NULL`;
  - `review_comment TEXT NULL` — комментарий последнего отклонения.
- CHECK: `status <> 'review' OR submitted_at IS NOT NULL` — на проверке всегда есть дата отправки.
- Индекс под ленту наставника: `CREATE INDEX intern_tasks_review_idx ON buddy.intern_tasks (submitted_at) WHERE status = 'review'`.
- Демо-данные (желательно, для показа): одну задачу Саши (`sasha.intern`, например «Доступ к Jira») перевести в `review` с `submitted_at = now() - interval '20 minutes'` — тогда у `anna_mentor` сразу есть запрос в блоке. Задачи создаются лениво (`EnsureTasks`), но у Саши они уже засеяны в 0006.

**down:** вернуть задачи из `review` в `in_progress`, удалить индекс и колонки, вернуть старый CHECK статуса.

**Тест:** `migrations_test.go` ждёт версию 7 → поменять на 8, после отката — 7.

---

## Шаг 2. Модель (`internal/onboarding/model.go`)

- Новый статус `StatusReview Status = "review"`; `Valid()` принимает 4 статуса.
- `Task` — два поля с `omitempty`:
  - `SubmittedAt *time.Time json:"submitted_at,omitempty"`;
  - `ReviewComment *string json:"review_comment,omitempty"`.
- Новые ошибки:
  - `ErrAlreadyApproved` — стажёр пытается менять принятую задачу;
  - `ErrNotInReview` — approve/reject задачи не в `review`;
  - `ErrCommentRequired` — reject без комментария.
- Тип для ленты наставника:
  ```go
  type Review struct {
      TaskID          int64     `json:"task_id"`
      TaskTitle       string    `json:"task_title"`
      TaskDescription string    `json:"task_description"`
      StageTitle      string    `json:"stage_title"`
      Intern          Intern    `json:"intern"`
      SubmittedAt     time.Time `json:"submitted_at"`
  }

  // Intern — кто отправил задачу; только id и имя (своя структура, без зависимости от postgres/buddy).
  type Intern struct {
      ID       int64  `json:"id"`
      FullName string `json:"full_name"`
  }
  ```

---

## Шаг 3. Хранилище (`repository.go` + `postgres.go`)

В интерфейс `Store` добавить три метода, в `Postgres` — реализацию.

**`ListTasks`** — дочитывать `submitted_at`, `review_comment` (в select и Scan).

**`UpdateStatus` (стажёр, меняется логика):**
- Не менять задачу в `done`: условие `and status <> 'done'` в `where`. Если строка не обновилась — отличить «нет такой/чужая» (`ErrNotFound`) от «уже принята» (`ErrAlreadyApproved`). Проще всего — `select status ... where id and user_id` перед апдейтом или CTE, который возвращает текущий статус.
- При переходе в `review`: `submitted_at = now()`, `review_comment = null` (новая попытка — старый комментарий больше не актуален).
- Повторный `review` для задачи, которая уже в `review`, — no-op: `submitted_at = case when status = 'review' then submitted_at else now() end`, ответ 200 (агент и двойной клик не перемешивают очередь наставника).
- При переходе в `todo`/`in_progress`: `submitted_at = null`, `review_comment` не трогать (стажёр взял задачу в работу после отклонения — комментарий нужен).
- `completed_at` для стажёра теперь всегда `null` (done он не ставит).

**`ListReviews(ctx, mentorID) ([]Review, error)`:**
- `intern_tasks` ⋈ `users` (стажёр, `u.mentor_id = $1`) ⋈ `task_templates` ⋈ `onboarding_stages`, `where it.status = 'review'`, `order by it.submitted_at, it.id`.
- Пустой результат — `[]Review{}`, не `nil` (иначе в JSON будет `null`).

**`Review(ctx, mentorID, taskID, approve bool, comment string) error`** (или два метода `Approve`/`Reject` — как удобнее):
- Один атомарный `update ... where it.id = $taskID and it.status = 'review' and it.user_id in (select id from buddy.users where mentor_id = $mentorID)`.
- approve: `status='done', completed_at=now(), reviewed_by, reviewed_at=now(), review_comment=null, submitted_at=null`.
- reject: `status='in_progress', review_comment=$comment, reviewed_by, reviewed_at=now(), submitted_at=null`.
- 0 строк → различить:
  - задачи нет **или стажёр не твой** → `ErrNotFound` (не раскрываем, что чужая задача существует);
  - задача твоего стажёра, но не в `review` → `ErrNotInReview`.
  Вторым запросом `select status from intern_tasks it join users u ... where it.id=$1 and u.mentor_id=$2`.
- Гонки: двойной клик «Принять» или одновременно approve и reject — второй запрос получит 0 строк → `ErrNotInReview` → 409. Отдельная блокировка не нужна, `where status='review'` атомарен.

---

## Шаг 4. Сервис (`service.go`)

- **`SetStatus`** (стажёр, HTTP и MCP идут через него):
  - `done` от стажёра → заменить на `review` **до** вызова store (и для модели, и для фронта);
  - ошибки store пробрасывать как есть (`ErrNotFound`, `ErrAlreadyApproved`).
- **`build()`**:
  - в прогресс (`Done`) считать только `done` — уже так;
  - `Next` — первая задача в `todo` или `in_progress`; `review` пропускать (сейчас берётся любая не-done).
- **`Reviews(ctx, mentorID)`** — тонкая обёртка над `ListReviews`.
- **`Approve(ctx, mentorID, taskID)`**, **`Reject(ctx, mentorID, taskID, comment)`**:
  - Reject: `strings.TrimSpace(comment)`; пусто → `ErrCommentRequired`; длиннее 500 рун → `ErrCommentRequired` (или отдельная ошибка — главное 422).

Роль в токене (`claims.Role`) для наставника **не проверяем**: доступ ограничен самим запросом (`mentor_id = я`). Не наставник просто получает пустой список и 404 на действия. Так проще и нечего обойти.

---

## Шаг 5. HTTP

Новый набор ручек — в `onboarding/http.go` (рядом с чек-листом) или отдельный `onboarding/reviews_http.go`; регистрируется тем же `Register`, отдельной правки `main.go` не нужно.

| Ручка | Успех | Ошибки |
|---|---|---|
| `GET /api/reviews` | `200 {"reviews":[Review]}` | 401, 500 |
| `POST /api/reviews/{id}/approve` | `200 {"task_id":42,"status":"done"}` | 400 invalid task id, 404 not found, 409 task is not in review, 500 |
| `POST /api/reviews/{id}/reject` body `{"comment":"…"}` | `200 {"task_id":42,"status":"in_progress"}` | 400 invalid task id / invalid json, 404, 409, 422 comment is required, 500 |

Изменения в существующем `PATCH /api/checklist/tasks/{id}`:
- `ErrAlreadyApproved` → `409 {"error":"task already approved"}`;
- текст 400 → `invalid status: use todo, in_progress, review or done`.

Тело reject — через `MaxBytesReader` (2 КБ хватит), как в `handleSetStatus`. В лог — только операцию, id пользователя/задачи и ошибку, без комментария.

---

## Шаг 6. MCP (`onboarding/mcp.go`)

Сигнатура инструментов **не меняется** — агент уже вызывает их так.

- `update_task_status`:
  - jsonschema поля `status`: `todo, in_progress или done (done = отправить наставнику на проверку)`;
  - Description: «…Стажёр не может сам завершить задачу: done превращается в review — задача уходит наставнику на проверку. В ответе фактический статус».
  - `ErrAlreadyApproved` → текст ошибки `task already approved`, `ErrNotFound` → `task not found` (агент сопоставляет коды и сам формулирует ответ; остальные ошибки БД скрываем, как сейчас).
- `get_checklist` — Description дополнить: «status review — на проверке у наставника; review_comment — почему наставник вернул задачу».
- Инструменты для наставника в чате **не делаем** (принимать/отклонять — только кнопками). Если будет время — read-only `list_reviews` (обёртка над `Reviews`) для вопроса наставника «что мне проверить?»; агенту его тоже придётся добавить в graph.py.

---

## Шаг 7. Тесты (держим покрытие > 80%)

На каждый слой — в стиле существующих (`postgres_test` на pgxmock, `service_test` на фейковом Store, `http_test` через httptest, `mcp_test`).

- **postgres:** ListTasks с новыми полями; UpdateStatus → review (submitted_at выставлен); UpdateStatus принятой задачи → ErrAlreadyApproved; ListReviews (пусто → `[]`); approve/reject успех; 0 строк → ErrNotFound / ErrNotInReview; ошибка БД.
- **service:** done от стажёра → в store уходит review; build: review не в прогрессе и не в next; reject с пустым/пробельным/длинным комментарием → ErrCommentRequired.
- **http:** все строки таблицы из шага 5, включая 401 без claims и 409 на PATCH принятой задачи.
- **mcp:** update_task_status(done) возвращает review; ErrAlreadyApproved → понятный текст.
- **миграции:** версия 8 и откат на 7.

После: `make cover` — общий процент не ниже 80.

---

## Шаг 8. Проверка руками (после `docker compose up -d --build api mcp-buddy migrator`)

1. `sasha.intern`: в чате «я получил доступ к GitLab» → на главной задача «На проверке», прогресс не вырос.
2. Выйти → `anna_mentor`: на главной «Запросы на проверку» — Саша / GitLab (+ демо-задача Jira из миграции).
3. «Отклонить» с комментарием → у Саши «Нужно доработать» и комментарий; Buddy на «что с GitLab?» пересказывает комментарий.
4. Снова отправить → у Анны «Принять» → у Саши «Готово», прогресс +1.
5. `petya.intern` не видит ничего от Саши; `elena_mentor` не видит запросы Саши (404 на чужой approve через curl).

---

## Что не входит (если останется время)

- Отгулы в том же блоке наставника (сейчас висят в `submitted` без согласования) — та же механика approve/reject.
- Уведомления (почта/мессенджер) — заявлено как «появится у наставника на главной», не обещаем больше.
- История проверок (сейчас храним только последнее решение).
