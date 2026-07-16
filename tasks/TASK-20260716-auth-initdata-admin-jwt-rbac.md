# TASK-20260716-auth-initdata-admin-jwt-rbac: Модуль auth (initData + admin JWT + RBAC)

**Type:** feature  
**Size:** L  
**Status:** todo
**GitHub Issue:** #5 — https://github.com/prog-time/tg-shop/issues/5

## Что делаем
Реализовать модуль `auth`: серверную проверку Telegram `initData` для витрины (`Authorization: tma <initData>`), аутентификацию персонала по email+пароль с выдачей access/refresh JWT для админки, и RBAC-роли (admin / order_manager / content_manager). Витрина и админка используют разные механизмы аутентификации (инвариант).

## Пользователь получает
Как **покупатель и как персонал**, я могу **безопасно обращаться к своим эндпоинтам**, чтобы **витрина работала по initData, а админка — по логину с ролями, и никто не получил чужой доступ**.

## Готово когда
- [ ] Given запрос витрины, when приходит `initData`, then подпись проверяется только на сервере по `BOT_TOKEN`; невалидная → `401`
- [ ] Given персонал, when `POST /admin/auth/login` с верными данными, then выдаются access + refresh JWT; `refresh`/`logout`/`me` работают
- [ ] Given RBAC, when роль не имеет права на операцию, then `403`; права проверяются в middleware
- [ ] Given `GET /me`, when валидный initData, then возвращается/создаётся запись покупателя (`users.telegram_id`)

## Как делаем
- Проверка `initData` по алгоритму Telegram (HMAC от `BOT_TOKEN`), с проверкой свежести.
- JWT на `golang-jwt/v5`, подпись `JWT_SECRET`; access короткий, refresh длинный.
- RBAC как middleware поверх сгенерированных операций; роли из `roles`, персонал из `admin_users`.

## Файлы
- `backend/internal/auth/**` — модуль
- `backend/internal/httpx/*` — middleware аутентификации/авторизации
- `❓ backend/internal/<gen>` — точки подключения security-схем

## Не делаем в этой задаче
- Доменные CRUD-операции админки (задача admin), UI логина.

## Вопросы
- ❓ TTL access/refresh и хранение refresh (stateless vs список отзыва).
- ❓ Хеш пароля (argon2id / bcrypt) — зафиксировать.

## Связанные задачи
- Зависит от: core-schema migrations, oapi-codegen + роутинг
- Блокирует: catalog, cart+media, orders, payment, admin

## Критерии приёмки (Definition of Done)
- [ ] Критерии из "Готово когда" выполнены и покрыты автотестами
- [ ] `go vet` и статический анализ (golangci-lint) зелёные
- [ ] Юнит- и интеграционные тесты (`go test ./...`) зелёные локально
- [ ] Документация (`docs/architecture.md` security, openapi при расхождении) обновлена
- [ ] Code review пройден
- [ ] Нет регрессий в смежных сценариях
