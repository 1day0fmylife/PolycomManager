# Polycom Manager — Plan

Цель: единый Go executable для Linux/Windows с embedded React/Vite UI для централизованного управления несколькими Polycom RealPresence Group 300 / 500 / 700 по Secure API over SSH.

## Текущая итерация

- [x] **1. Camera/PTZ + presets**
  - near/far camera selection;
  - PTZ: left/right/up/down/zoom+/zoom-/stop;
  - near presets 0..99, far presets 0..15;
  - UI учитывает различия Group 300/500/700 по доступным near camera inputs.
- [x] **2. DTMF keypad**
  - `0..9`, `*`, `#` во время активного вызова;
  - typed REST endpoint вместо raw API command из браузера.
- [x] **3. Content Sharing**
  - start/stop;
  - выбор content source;
  - runtime state `play/stop`, текущий source;
  - модельные ограничения для Group 300/500/700.
- [x] **4. Расширенный экран активного вызова**
  - все call legs из `callinfo all`;
  - Call ID, far-site name/number, bitrate, state, mute, direction, type;
  - protocol hint (SIP/H.323/IP, когда его можно безопасно определить из адреса);
  - наблюдаемая длительность call leg;
  - глобальные mute/volume/hangup и DTMF.
- [x] **8. Dashboard / мониторинг парка**
  - online/total;
  - количество active call legs;
  - content sharing count;
  - offline/reconnecting/auth errors;
  - активные вызовы по терминалам;
  - распределение по моделям;
  - список устройств, требующих внимания.

## Следующие этапы

- [ ] **5. Multipoint / conference control**
  - отдельное завершение call leg;
  - приглашение дополнительного участника;
  - UI списка участников и состояний каждого leg.
- [ ] **6. System Control**
  - reboot/wake/sleep;
  - uptime/health;
  - SIP/H.323 registration state;
  - дополнительные system/network сведения.
- [ ] **7. Remote UI / remote-control**
  - текущее состояние экрана;
  - поддерживаемые navigation/button actions;
  - безопасная панель remote control.
- [ ] **9. Group operations**
  - multi-select терминалов;
  - reconnect/mute/reboot/health-check для группы;
  - batch result и audit.
- [ ] **10. Configuration profiles**
  - профили «Переговорная», «Большой зал», «Учебный класс»;
  - capability-aware применение настроек;
  - preview/diff перед применением.
- [ ] **11. Event Center**
  - расширить event-driven state вместо polling там, где API даёт notifications;
  - call/content/camera/preset/system events;
  - история событий и фильтрация.
- [ ] **12. Security / multi-user**
  - login/session;
  - RBAC `viewer/operator/admin`;
  - ограничение Raw API Console;
  - TLS deployment guidance;
  - расширенный audit: кто/когда/что выполнил.

## Технические принципы

1. Browser никогда не получает SSH credentials и не подключается к терминалу напрямую.
2. На каждый терминал — отдельный долгоживущий SSH/API worker; команды внутри одной API-сессии сериализуются.
3. Специализированные операции идут через typed backend methods/endpoints; Raw API Console остаётся диагностическим инструментом.
4. Capability UI и backend validation должны учитывать модель и фактически обнаруженный терминал.
5. React bundle встраивается через `go:embed`; `make build` всегда пересобирает и проверяет UI до компиляции Go.
6. Распространение — один executable на платформу и отдельный data directory с SQLite/master key/known host keys.

## Источник команд Polycom

Команды сверяются с **Polycom RealPresence Group Series Integrator Reference Guide v5.0.0**, опубликованным HP/Poly. В частности: `camera`, `preset`, `gendial`, `vcbutton`, `callinfo`.
