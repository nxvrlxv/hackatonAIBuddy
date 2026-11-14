# AI Buddy: фронтенд

Интерфейс стажёра и наставника: вход, «Главная» и «AI-чат». Все данные идут из Go API (`backend/cmd/api`): вход, профиль, чек-лист, команда, проверка задач наставником и чат. Заглушка `src/api/mock.ts` нужна только для режима `mock`.

Стек: React 19, TypeScript, Vite 8, react-markdown, шрифт Inter (`@fontsource-variable/inter`).

## Запуск

В составе стенда интерфейс поднимается из корня репозитория командой `docker compose up --build` и открывается на http://localhost:3000. Образ собирается в `node:25-alpine`, nginx раздаёт сборку и проксирует `/api` на Go API без буферизации (`Dockerfile`, `nginx.conf`).

Для разработки нужен Node.js 20+.

```bash
npm install
npm run dev        # с настоящим бэкендом: /api проксируется на http://localhost:8080
npm run dev:mock   # без бэкенда: данные и ответ Buddy из заглушек
```

Откроется http://localhost:5173. Бэкенд поднимается из корня репозитория: `docker compose up --build -d api mcp-buddy agent` (подробно в `docs/docker-compose.md`). «Войти через Avito ID» входит тестовой учёткой `sasha.intern` без пароля (имитация SSO, другая учётка задаётся `VITE_DEMO_LOGIN`). «Войти по email» принимает любой тестовый логин, например `anna_mentor`; список в корневом README. Переменные в `.env.example`.

Сборка: `npm run build`, результат в `dist/`. Проверка типов — `npx tsc -b`, линт — `npm run lint`, тесты — `npm test`. Покрытие — `npm run coverage`: считается по всем файлам `src` (настройка `coverage.include` в `vite.config.ts`), HTML-отчёт в `coverage/index.html`.

Тесты лежат рядом с кодом: `src/api/*.test.ts` проверяют сеть и поток SSE в окружении node, `src/*.test.tsx` проверяют экраны в jsdom через Testing Library (строка `// @vitest-environment jsdom` в начале файла). Сеть в тестах экранов подменяется заглушками `src/api/client.ts`.

## Устройство

- `src/App.tsx`: переключение экранов (вход → главная ↔ чат).
- `src/Login.tsx`, `src/Home.tsx`, `src/Chat.tsx`: экраны.
- `src/Reviews.tsx`, `src/Interns.tsx`: «Запросы на проверку» и «Мои стажёры» на главной наставника.
- `src/Help.tsx`: окно «Что умеет Buddy» с примерами вопросов.
- `src/Markdown.tsx`: разметка ответов Buddy (сырой HTML не исполняется).
- `src/Shell.tsx`: боковая панель, вкладки и меню пользователя.
- `src/ui.tsx`: иконки, логотип, аватар, маскот.
- `src/index.css`: токены, размеры, цвета, анимации и адаптив.
- `src/api/client.ts`: **единственное место, где есть сеть**: вход, главная, проверка задач, история чата и потоковый ответ Buddy (SSE). Режим `mock` отвечает без сети.
- `src/api/sse.ts`: разбор потока Server-Sent Events.
- `src/api/types.ts`: контракты (`User`, `ChatMessage`, `HomeData`, …).
- `public/icons/`: SVG-иконки из макета (`01-email.svg` … `20-hr-calendar-blue.svg`, `avito-mark.svg`).
- `public/img/`: логотип и три маскота WebP с прозрачным фоном (обрезаны по краю картинки: позиции в CSS посчитаны под эту обрезку, поля добавлять нельзя).

Вёрстка адаптивная и не привязана к координатам макета: две колонки на десктопе, одна на планшете, горизонтальная панель на телефоне.

### Иконки

Файлы в `public/icons/` используются как CSS-маска (`Icon` в `src/ui.tsx`), поэтому цвет задаётся через `currentColor`. Исходные заливки внутри SVG игнорируются — это позволяет красить одну и ту же иконку в белый на цветных кружках и в тёмный в тексте.

## Бэкенд

Все запросы собраны в `src/api/client.ts`:

| Функция | Ручка Go API |
| --- | --- |
| `login()` | `POST /api/login` `{login}` → `{token, user}` |
| `fetchMe()` | `GET /api/me` |
| `fetchHistory()` | `GET /api/conversations`, затем `GET /api/conversations/{id}/messages` последнего чата |
| `sendMessage(text)` | `POST /api/conversations` (если чата ещё нет), затем `POST /api/chat` — поток SSE |
| `fetchHome()` | `GET /api/me`, `GET /api/checklist`, `GET /api/team` параллельно |
| `submitForReview(id)` | `PATCH /api/checklist/tasks/{id}` `{status: "review"}` |
| `fetchReviews()` | `GET /api/reviews` (наставник) |
| `approveTask(id)`, `rejectTask(id, comment)` | `POST /api/reviews/{id}/approve`, `POST /api/reviews/{id}/reject` `{comment}` |
| `fetchInterns()` | `GET /api/interns` (наставник) |

Токен хранится в `sessionStorage` и уходит в заголовке `Authorization: Bearer`. Ответ 401 возвращает на экран входа.

Поток `/api/chat`: `status` показывается в шапке чата, `text_delta` печатается в пузыре по мере прихода, `done` завершает ответ. При событии `error`, обрыве потока или тишине дольше 180 секунд (`IDLE_TIMEOUT_MS`) под вопросом появляется ошибка с кнопкой «Повторить»; интерфейс при этом не блокируется. Порог большой: пока модель на CPU решает, вызвать ли инструмент, событий нет до 2,5 минуты. «Начать заново» открывает новый чат на бэкенде.

Статусы задач и правила проверки наставником описаны в `docs/approvals-contract.md` и `docs/specs/02-onboarding.md`.

## Отладка

- `?s=main` или `?s=chat` открывает нужную вкладку после входа (в режиме `mock` — сразу).
- В режиме `mock` роль задаётся адресом: `?as=mentor` — наставник с запросами на проверку, `?as=lead` — руководитель.
- Исходные кадры макета лежат в `public/ref/` (`p1`–`p3`) для сверки глазами.
