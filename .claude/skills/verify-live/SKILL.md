---
name: verify-live
description: Use when a change must be proven in the running launcher, not just in tests — a frontend or UI edit, a flow that only shows in the window, "проверь в живом окне", screenshot the app, run the devmock build on macOS, rebuild and restart on Windows, or when the window shows no change after an edit.
---

# Живая проверка лаунчера

Чеклист в CLAUDE.md запрещает «судя по diff». Доказательство для UI — скриншот окна той
сборки, в которую попала правка, плюс pid процесса в отчёте. Этот скилл — как получить
такой скриншот и не потратить час на ловушки.

Гонять окно можно только по явной просьбе и одним коротким заходом: каждый `set frontmost`
выдёргивает пользователя из его работы. Если клики не долетают со второй попытки —
не долбить, а вынести логику в модуль и покрыть юнит-тестом.

## macOS

### 0. Инструменты

```bash
export PATH="$HOME/go/bin:$PATH"                       # wails3 и golangci-lint лежат там
sh .claude/skills/verify-live/scripts/build.sh "$UI"   # UI=папка в скретчпаде; даёт winid, click, scroll
```

`build.sh` компилирует три файла `scripts/*.swift` через `swiftc` (нужны Xcode CLT, ~10 секунд).
Хелперы нужны потому, что окно нативное: Chrome-инструменты до него не дотягиваются.

`winid` печатает окна лаунчера (`winid all` — включая другие Spaces), `click X Y` кликает
по экранным координатам, `scroll X Y TICKS` крутит колесо (отрицательные — вниз; страница
скроллится div'ом, PageDown не работает).

### 1. Убить старый экземпляр

```bash
ps -eo pid,lstart,command | grep '[b]in/typhon'
pkill -f 'bin/typhon'; sleep 1
ps -eo pid,command | grep '[b]in/typhon' && pkill -9 -f 'bin/typhon'   # не умер по SIGTERM
```

Лаунчер single-instance: если старый жив, новый бинарь молча передаёт ему фокус и выходит,
и ты смотришь на старую сборку. Проверять до каждого запуска, не только в первый раз.

### 2. Собрать и запустить

```bash
wails3 task build:devmock && ls -la bin/typhon         # время файла — после правки
find frontend/dist -newer frontend/src/App.svelte -name '*.js' | head -2   # бандл собран после правки (подставь свой файл)
TYPHON_API_URL=http://127.0.0.1:8080 ./bin/typhon &    # локальный бэкенд
./bin/typhon &                                         # без переменной — прод
sleep 3; tail -5 "$HOME/Library/Application Support/typhon/typhon.log"
```

Какой бэкенд нужен, решает задача: локальный (`typhon-backend-feed`, засеянный каталог,
логин `egor`) для фич с сервером, прод — когда надо видеть настоящие данные. В логе при
старте строка `devmock build: Windows-only subsystems are mocked`; `invalid_credentials`
в первые секунды — нормальный хендшейк. Фоновый `wails3 task dev:devmock` держит свой
`TYPHON_API_URL` из момента запуска — пересборка через него вернёт старый бэкенд.

### 3. Найти окно и подготовить его

```bash
"$UI/winid"                                            # id=… pid=… x=0 y=30 w=2560 h=1318
osascript -e 'tell application "System Events" to tell (first process whose name contains "typhon") to tell window 1 to set size to {2560, 1318}'
osascript -e 'tell application "System Events" to set frontmost of (first process whose name contains "typhon") to true'
```

Окно без Retina-масштаба: точки = пиксели, окно начинается на y=30, поэтому
**экранный y = y на скриншоте + 30**. `set frontmost` — перед каждой серией кликов, иначе
события уходят в другое окно. Если `winid` без `all` окно не видит — оно на другом Space
(терминал во весь экран): скриншот снять можно, кликнуть нельзя.

### 4. Кликнуть и снять

Сайдбар при 2560x1318, x=104: библиотека 139, каталог 185, установленные 231, загрузки 296,
друзья 342, лента 388, профиль 434, настройки 481, «О программе» 1246. После правок
сайдбара координаты перепроверить по скриншоту.

```bash
"$UI/click" 104 481; sleep 1.8
screencapture -o -x -l"$ID" "$OUT/settings.png"       # только окно
screencapture -o -x -R 883,253,204,83 "$OUT/card.png"   # кусок экрана, координаты экранные
```

Скриншот прочитать инструментом Read и сверить с диффом глазами: нужный текст, нужное
состояние. Один кадр на проверяемое утверждение.

### 5. Отчёт

pid процесса, время файла `bin/typhon`, путь к скриншоту, что на нём подтверждает правку.
Если правки не видно — по порядку: время бинаря старше правки; `ps` показывает второй
экземпляр; скриншот снят с другого id (у лаунчера несколько окон, нужное — с `h=1318`);
frontend не пересобрался (`frontend/dist`).

### Перед тестами

Запущенный лаунчер держит UDP-порт LAN-обнаружения: `go test ./internal/lan/` падает с
`bind: address already in use`. Это не регрессия — `pkill -f bin/typhon` и повторить.
Зависший тест с `panic: test timed out after 10m` — см. память про `Skip`/`Fatal` под
мьютексом: в дампе горутина стоит в `Mutex.Lock`, держателя нет.

### Где смотреть, когда не работает

`~/Library/Application Support/typhon/`: `typhon.log` (с ротацией `.1`…), состояние —
`downloads.json`, `installation.json`, `catalog.json`, `account.json`, `compat*.json`,
фейковые процессы devmock — `devmock-processes.json`. Самообновление целиком:
`wails3 task devrelease VERSION=x.y.z` и две переменные, которые он печатает
(раздел про devmock в CLAUDE.md).

## Windows

```powershell
wails3 task build
taskkill /IM typhon.exe /F; Start-Process .\bin\typhon.exe
Get-Content $env:APPDATA\Typhon\typhon.log -Tail 20      # свежий хвост, без паник
```

Бэкенд, если правка его задевает: перезапустить `go run ./cmd/api` в `typhon-backend`,
дождаться `GET /ready` = 200 на `127.0.0.1:8080`, убедиться, что порт держит новый pid.
Скриншот окна — через инструменты Claude in Chrome недоступен (окно нативное); на Windows
проверка — лог плюс ручной взгляд пользователя, в отчёте pid и хвост лога.

## Частые ошибки

- Запустил новую сборку, не убив старую, и час искал, почему правки нет.
- `wails3 task build` на маке вместо `build:devmock` — без моков Windows-подсистем окно не поднимется до нужного экрана.
- Клик до `set frontmost` — событие ушло в терминал.
- Скриншот с окна-полоски высотой 30 вместо главного.
- Пять заходов кликами вместо одного юнит-теста.
