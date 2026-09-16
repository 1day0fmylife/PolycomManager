# Polycom Manager

MVP для централизованного управления несколькими **Polycom RealPresence Group 300 / 500 / 700** по документированному Secure API over SSH.

- редактирование сохранённых терминалов с сохранением текущего пароля при пустом поле;

## Что реализовано

- хранение нескольких терминалов в SQLite;
- автоматическое восстановление терминалов после перезапуска;
- отдельный SSH worker и долгоживущая API-сессия на каждое устройство;
- reconnect с exponential backoff;
- остановка автоматического reconnect при ошибке SSH-аутентификации;
- TOFU SSH host-key store с обнаружением изменившегося ключа;
- зашифрованное хранение SSH-паролей (XChaCha20-Poly1305);
- автоматическое определение модели через `systemsetting get model`;
- получение system name, firmware, serial и состояния вызова;
- `dial`, `hangup`, `mute`, `volume`;
- Raw API console;
- уведомления `callstatus`, `mutestatus`, `sysstatus`;
- журнал аудита;
- REST API + Server-Sent Events;
- React/Vite UI, встраиваемый в Go через `go:embed`;
- один executable на платформу: Linux amd64 и Windows amd64;
- pure-Go SQLite (`modernc.org/sqlite`), поэтому `CGO_ENABLED=0`.

## Требования для сборки

- Go 1.23+;
- Node.js 22+ и npm для пересборки React UI;
- доступ к Go module proxy/npm registry при первой установке зависимостей.

## Быстрый старт для разработки

Полная локальная сборка теперь выполняется одной командой:

```bash
make build
```

`make build` последовательно выполняет `go mod tidy`, устанавливает npm-зависимости, собирает Vite, проверяет `web/dist/index.html`, атомарно копирует весь bundle в `internal/webui/dist` и только после этого собирает Go executable. Это предотвращает сборку бинарника без Web UI.

Запуск:

```bash
./dist/polycom-manager --data-dir ./data --listen 127.0.0.1:8080
```

Если Web UI уже подготовлен и нужно пересобрать только Go-код:

```bash
make build-go
```

Отдельно пересобрать UI:

```bash
make web
```

После запуска откроется `http://127.0.0.1:8080`.

> `make build` всегда пересобирает полноценный React/Vite UI перед компиляцией Go. Target `verify-web` дополнительно проверяет, что `internal/webui/dist/index.html` и остальные assets действительно присутствуют.

## Release build

Linux/macOS shell:

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
./build-release.ps1
```

Результат:

```text
dist/
├── polycom-manager-linux-amd64
└── polycom-manager-windows-amd64.exe
```

Оба файла включают backend и web UI. Рядом с executable не требуется Node.js, nginx или каталог `dist`.

## Использование

```bash
./polycom-manager-linux-amd64 \
  --listen 127.0.0.1:8080 \
  --data-dir ./data
```

Флаги:

```text
-listen     HTTP address, default 127.0.0.1:8080
-data-dir   directory for DB/master key/SSH host keys
-open-ui    open browser automatically, default true
-log-level  debug|info|warn|error
```

### Где хранятся данные по умолчанию

Linux root:

```text
/var/lib/polycom-manager/
```

Linux user:

```text
~/.local/share/polycom-manager/
```

Windows:

```text
%PROGRAMDATA%\PolycomManager\
```

Состав:

```text
polycom-manager.db
master.key
hostkeys.json
```

## Master key

По умолчанию при первом запуске создаётся `master.key` с правами `0600` (на POSIX). Для production предпочтительнее передать 32-байтный ключ в base64 через переменную окружения:

```bash
export POLYCOM_MANAGER_MASTER_KEY='BASE64_32_BYTE_KEY'
```

При использовании env key локальный `master.key` не создаётся.

## Polycom API

MVP использует команды семейства RealPresence Group Series:

```text
session name <name>
notify callstatus
notify mutestatus
notify sysstatus
systemsetting get model
systemname get
version
serialnum
callinfo all
mute near get
mute near on|off
volume get
volume set 0..50
dial manual <speed> "<destination>" sip
hangup all
```

Для вызова используется SIP и скорость по умолчанию `512`.

На терминале должен быть разрешён Secure/Legacy API over SSH. SSH port по умолчанию — `22`.

## REST API

```text
GET    /api/v1/health
GET    /api/v1/devices
POST   /api/v1/devices
GET    /api/v1/devices/{id}
PUT    /api/v1/devices/{id}
PATCH  /api/v1/devices/{id}
DELETE /api/v1/devices/{id}
POST   /api/v1/devices/{id}/connect
POST   /api/v1/devices/{id}/disconnect
POST   /api/v1/devices/{id}/dial
POST   /api/v1/devices/{id}/hangup
POST   /api/v1/devices/{id}/mute
POST   /api/v1/devices/{id}/volume
POST   /api/v1/devices/{id}/command
GET    /api/v1/audit
GET    /api/v1/events
```

Пример добавления:

```json
{
  "name": "Conference Room 101",
  "host": "192.168.100.31",
  "port": 22,
  "model": "auto",
  "location": "Room 101",
  "username": "admin",
  "password": "secret",
  "enabled": true
}
```

## Безопасность

По умолчанию HTTP слушает только loopback (`127.0.0.1`). Если интерфейс публикуется в LAN, перед эксплуатацией следует добавить аутентификацию/RBAC и TLS reverse proxy. Raw API Console даёт оператору возможность выполнять команды на терминале, поэтому её нельзя публиковать без контроля доступа.

SSH host keys работают по TOFU: первый увиденный ключ сохраняется в `hostkeys.json`, а его последующая замена приводит к ошибке подключения. Для сознательной замены ключа удалите соответствующую запись из файла после проверки нового fingerprint.

## Структура

```text
cmd/polycom-manager/       entrypoint
internal/api/              REST + SSE
internal/app/              wiring/lifecycle
internal/config/           flags/data directory
internal/crypto/           credential encryption
internal/db/               SQLite repository/migrations
internal/device/           manager + per-device workers
internal/events/           event hub
internal/polycom/          Group Series driver/parser
internal/sshclient/        SSH session + host key store
internal/webui/            embedded UI
web/                       React/Vite source
```

## Ограничения MVP

- SSH password auth; private-key auth пока не добавлен.
- Web terminal реализован как безопасная Raw API Console, а не полноценный PTY/xterm multiplexing.
- Нет встроенной пользовательской аутентификации/RBAC; приложение по умолчанию loopback-only.
- Парсер call info рассчитан на документированный формат Group Series и должен быть проверен на ваших конкретных firmware versions.
- Для multipoint UI сейчас показывает первый активный call; API parser возвращает все call entries и его легко расширить.
