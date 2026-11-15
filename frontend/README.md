# AI Buddy: фронтенд

Интерфейс стажёра: вход, «Главная» и «AI-чат». Вход, профиль и чат работают через Go API (`backend/cmd/api`), онбординг и контакты пока из заглушки `src/api/mock.ts`.

Стек: React 19, TypeScript, Vite 8, шрифт Inter (`@fontsource-variable/inter`).

## Запуск

Нужен Node.js 20+.

```bash
npm install
npm run dev        # с настоящим бэкендом: /api проксируется на http://localhost:8080
npm run dev:mock   # без бэкенда: данные и ответ Buddy из заглушек
```

Откроется http://localhost:5173. Бэкенд поднимается из корня репозитория: `docker compose up --build -d api mcp-buddy agent` (подробно в `docs/docker-compose.md`). Вход по кнопкам «Войти через Avito ID» и «Войти по email» — тестовая учётка `sasha.intern` без пароля (имитация SSO, как в Go API); другая учётка — через `VITE_DEMO_LOGIN`. Переменные — в `.env.example`.

Сборка: `npm run build`, результат в `dist/`. Проверка типов — `npx tsc -b`, линт — `npm run lint`, тесты — `npm test`, покрытие — `npm run coverage`.

## Устройство

- `src/App.tsx`: переключение экранов (вход → главная ↔ чат).
- `src/Login.tsx`, `src/Home.tsx`, `src/Chat.tsx`: экраны.
- `src/Shell.tsx`: боковая панель и вкладки.
- `src/ui.tsx`: иконки, логотип, аватар, маскот.
- `src/index.css`: токены, размеры, цвета, анимации и адаптив.
- `src/api/client.ts`: **единственное место, где есть сеть**: вход, профиль, история чата и потоковый ответ Buddy (SSE). Режим `mock` отвечает без сети.
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
| `fetchHome()` | пока заглушка: у бэкенда нет ручек онбординга и контактов |

Токен хранится в `sessionStorage` и уходит в заголовке `Authorization: Bearer`. Ответ 401 возвращает на экран входа.

Поток `/api/chat`: `status` показывается в шапке чата, `text_delta` печатается в пузыре по мере прихода, `done` завершает ответ. При событии `error`, обрыве потока или тишине дольше 30 секунд под вопросом появляется ошибка с кнопкой «Повторить»; интерфейс при этом не блокируется. «Начать заново» открывает новый чат на бэкенде.

## Отладка

- `?s=main` или `?s=chat` открывает нужную вкладку после входа (в режиме `mock` — сразу).
- Исходные кадры макета лежат в `public/ref/` (`p1`–`p3`) для сверки глазами.
