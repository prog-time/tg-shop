# TASK-20260716-openapi-codegen-and-routing: oapi-codegen + каркас роутинга

**Type:** feature  
**Size:** M  
**Status:** todo
**GitHub Issue:** #4 — https://github.com/prog-time/tg-shop/issues/4

## Что делаем
Завести контракт-first пайплайн: `go generate` через oapi-codegen из `docs/api/openapi.yaml` (модели + strict chi-server интерфейс), смонтировать сгенерированный роутер в `api`, добавить общие middleware (request-id/correlation-id, логирование, восстановление паник) и единый маппинг доменных ошибок в схему ошибки из спеки.

## Пользователь получает
Как **разработчик бэкенда**, я могу **реализовывать хендлеры как методы сгенерированного интерфейса**, чтобы **код и контракт не расходились, а бойлерплейт не писался руками**.

## Готово когда
- [ ] Given `docs/api/openapi.yaml`, when запускается `go generate ./...`, then в `internal/openapi` генерируются модели и `StrictServerInterface` без ошибок
- [ ] Given сгенерированный роутер, when он смонтирован в `api`, then неимплементированные операции отвечают явным `501`, а не паникуют
- [ ] Given любой запрос, when он проходит через middleware, then есть correlation-id в логах и единый формат ошибки при сбое
- [ ] Given security-схемы из спеки, when вызывается защищённый путь, then подключены точки применения `initData`/`adminJWT` (сама проверка — в задаче auth)

## Как делаем
- `oapi-codegen.yaml` уже готов; закоммитить сгенерированный код (воспроизводимость сборки).
- chi-middleware: recover, request-id, slog-логирование, CORS не нужен (один origin).
- Единый `problem`/`error` writer по схеме ошибки из контракта.

## Файлы
- `backend/internal/openapi/openapi.gen.go` — сгенерированный код (в git)
- `backend/internal/httpx/*` — middleware, error-writer
- `backend/cmd/api/main.go` — монтирование роутера

## Не делаем в этой задаче
- Реализацию доменных хендлеров и аутентификацию (отдельные задачи).

## Вопросы
- ❓ Стратегия «не реализовано» → `501` vs скрытие пути до готовности модуля.

## Связанные задачи
- Зависит от: walking skeleton
- Блокирует: auth, catalog, cart+media, orders, payment, admin

## Критерии приёмки (Definition of Done)
- [ ] Критерии из "Готово когда" выполнены и покрыты автотестами
- [ ] `go vet` и статический анализ (golangci-lint) зелёные
- [ ] Юнит- и интеграционные тесты (`go test ./...`) зелёные локально
- [ ] Документация (backend/README про `go generate`) обновлена
- [ ] Code review пройден
- [ ] Нет регрессий в смежных сценариях
