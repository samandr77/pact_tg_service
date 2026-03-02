# pact_tg_service

gRPC-сервис для управления несколькими Telegram-соединениями через единый API.

## Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- [protoc](https://grpc.io/docs/protoc-installation/) + плагины:
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```
- [golangci-lint v2+](https://golangci-lint.run/welcome/install/)
- [grpcurl](https://github.com/fullstorydev/grpcurl) (для ручного тестирования)
- Telegram API credentials: `APP_ID` и `APP_HASH` с [my.telegram.org/apps](https://my.telegram.org/apps)

## Быстрый старт

```bash
# Кодогенерация (если изменялся proto)
make gen

# Сборка
go build -o server ./cmd/server/

# Запуск
APP_ID=<your_app_id> APP_HASH=<your_app_hash> ./server
```

Переменные окружения:

| Переменная | Описание | По умолчанию |
|---|---|---|
| `APP_ID` | Telegram App ID (обязательный) | — |
| `APP_HASH` | Telegram App Hash (обязательный) | — |
| `GRPC_ADDR` | Адрес gRPC-сервера | `:50051` |
| `APP_ENV` | Среда запуска: `production` / `development` | `development` |

## gRPC API

### CreateSession

Создаёт новое Telegram-соединение и возвращает QR-код для авторизации.

```bash
grpcurl -import-path proto -proto telegram.proto \
  -plaintext -d '{}' localhost:50051 \
  pact.telegram.TelegramService/CreateSession
```

```json
{
  "sessionId": "9430d376a83d7ed09a26e88d8bee123c",
  "qrCode": "tg://login?token=..."
}
```

Для сканирования QR:
```bash
qrencode -o qr.png "<qrCode value>" && open qr.png
```
В Telegram: **Settings → Devices → Link Desktop Device** → навести камеру на изображение.

---

### DeleteSession

Останавливает соединение и выполняет `auth.logOut` если сессия авторизована.

```bash
grpcurl -import-path proto -proto telegram.proto \
  -plaintext -d '{"session_id": "<session_id>"}' localhost:50051 \
  pact.telegram.TelegramService/DeleteSession
```

---

### SendMessage

Отправляет сообщение через указанную сессию. Сессия должна быть авторизована.

```bash
grpcurl -import-path proto -proto telegram.proto \
  -plaintext -d '{
    "session_id": "<session_id>",
    "peer": "@username",
    "text": "Hello!"
  }' localhost:50051 \
  pact.telegram.TelegramService/SendMessage
```

```json
{
  "messageId": "362555"
}
```

`peer` — юзернейм (`@username`) или числовой ID. Для отправки себе: `"peer": "me"`.

---

### SubscribeMessages

Серверный стрим входящих сообщений. Держит соединение открытым до отключения клиента.

```bash
grpcurl -import-path proto -proto telegram.proto \
  -plaintext -d '{"session_id": "<session_id>"}' localhost:50051 \
  pact.telegram.TelegramService/SubscribeMessages
```

```json
{
  "messageId": "362504",
  "from": "1121005707",
  "text": "Привет!",
  "timestamp": "1772465267"
}
```

## Архитектурные решения

### Изоляция сессий

Каждая сессия — независимый `telegram.Client` со своей горутиной, контекстом и каналом сообщений. `SessionManager` хранит все активные сессии в памяти под `sync.RWMutex`. При остановке сервера корневой контекст отменяется, что каскадно завершает все сессии.

### QR flow

1. Создаётся `telegram.Client` с `UpdateDispatcher`.
2. `qrlogin.OnLoginToken(dispatcher)` регистрирует хендлер и возвращает канал `loggedIn`.
3. В горутине запускается `client.Run`, внутри которого вызывается `QR.Auth` — он блокируется до сканирования QR.
4. Callback `showCallback` передаёт URL токена через буферизованный канал `qrURLCh` в `Create`.
5. После сканирования Telegram отправляет `UpdateLoginToken` → `QR.Auth` завершается → сессия помечается как авторизованная.

### Буферизованный канал обновлений

`session.msgCh` имеет буфер 100 сообщений. Это позволяет `UpdateDispatcher` продолжать принимать обновления от Telegram, даже если gRPC-клиент временно не читает стрим. При переполнении новое сообщение отбрасывается с предупреждением в лог.

## Проверка

```bash
make check   # lint + test + build
```
