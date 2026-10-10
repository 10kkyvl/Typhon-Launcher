---
name: macos
description: Use when working on the launcher on macOS or on mac-specific code — devmock build and its checklist, CrossOver and internal/wine (bottles, shared Steam bottle, session detection, Stop), Keychain, tray and window close on macOS, *_darwin.go / *_devmock.go files and build tags, the mac self-update bundle or the macOS release job.
---

# Лаунчер на macOS

Целевая платформа — Windows; всё, что ниже, нужно, когда работа идёт на маке или трогает
маковый код. Общие правила (инварианты, чеклист, очередь тяжёлых команд) — в `CLAUDE.md`,
здесь только то, что добавляется к ним на macOS. Тяжёлые команды из маковского чеклиста
идут через ту же очередь `.claude/heavy.sh`.

## macOS: разработка и тесты через `devmock`

Целевая платформа — Windows, но разрабатывать и прогонять лаунчер можно на macOS. Windows-only
подсистемы (раннер установщиков, процессы игр, детект процессов, хранилище учётных данных, ярлыки)
заменяются моками за build-тегом `devmock`.

- Мок-файлы: `*_devmock.go` с `//go:build devmock && !windows`. Заглушки для не-Windows, у которых
  есть мок-аналог, помечены `!windows && !devmock`. Общий пакет — `internal/devmock` (реестр
  фейковых процессов `<ConfigDir>/devmock-processes.json`, флаг `devmock.Enabled`).
- В exe для пользователя моки не попадают физически: тег `!windows` на файлах плюс
  `internal/devmock/forbid_devmock.go`, который не компилируется при `GOOS=windows` или теге
  `production`. Релизный workflow дополнительно проверяет, что `bin/typhon.exe` не содержит маркер
  `TYPHON_DEVMOCK_ENABLED`.
- Задачи: `wails3 task build:devmock`, `wails3 task run:devmock`, `wails3 task dev:devmock`
  (hot reload, конфиг `build/devmock.yml`), `wails3 task test:devmock`. Бэкенд —
  `TYPHON_API_URL=http://127.0.0.1:8080` на локальный `typhon-backend`, либо прод по умолчанию.
- Время жизни фейкового процесса игры — `TYPHON_DEVMOCK_GAME_SECONDS` (по умолчанию 60). Сессия
  закрывается либо по `StopGame`, либо по истечении времени через цикл детекта.
- Порог автоматического «Отошёл» — `TYPHON_DEVMOCK_AWAY_SECONDS` (по умолчанию 10 минут, как в
  релизе). Только в devmock-сборке: иначе статус не проверить, не просидев десять минут без ввода.
- Окно в devmock-сборке называется «Typhon [devmock]», в «О программе» рядом с платформой
  показывается `devmock`, в логе при старте — `devmock build: Windows-only subsystems are mocked`.
- Установщики exe/msi в devmock идут через тот же протокол повышенного воркера, что и на
  Windows: лаунчер пишет spec, запускает сам себя как `--install-worker <spec>` без прав,
  опрашивает state, отмена — через cancel-файл, живой воркер после перезапуска подхватывается в
  `ServiceStartup`. `TYPHON_DEVMOCK_ELEVATE=0` выключает воркер (прямая фейковая установка),
  `TYPHON_DEVMOCK_INSTALL_SECONDS` (по умолчанию 2) задаёт длительность фейковой установки, чтобы
  прогресс и отмена были видны. Лог воркера — `worker-<id>.log` рядом с state-файлом.
- Самообновление целиком: `wails3 task devrelease VERSION=0.3.1` собирает лаунчер с этой версией
  как артефакт, подписывает манифест одноразовым ключом и поднимает сервер на 127.0.0.1:8099;
  он печатает две переменные, `TYPHON_DEVMOCK_MANIFEST_URL` и `TYPHON_DEVMOCK_RELEASE_PUBKEY`,
  с ними запускается devmock-лаунчер (остальной API по-прежнему `TYPHON_API_URL`). Воркер
  обновления (`--selfupdate-worker`) ждёт выхода родителя, подменяет бинарь по rename и
  перезапускает; стадии — в `selfupdate/worker/progress.json` и `worker.log`, результат на
  следующем старте — в «О программе».

Чеклист на macOS (в дополнение к общему; `-race` здесь не требует ничего, кроме Xcode CLT).
Каждая строка идёт отдельным вызовом `bash .claude/heavy.sh <команда>`; цепочки через `&&`
ниже разбиты именно поэтому, а `for` оборачивается в `bash .claude/heavy.sh sh -c '…'`:

```bash
gofmt -l .
bash .claude/heavy.sh go vet . ./internal/...
bash .claude/heavy.sh go vet -tags devmock . ./internal/...
bash .claude/heavy.sh go build . ./internal/...
bash .claude/heavy.sh go build -tags devmock . ./internal/...
bash .claude/heavy.sh GOOS=windows go build . ./internal/...
bash .claude/heavy.sh sh -c 'for p in install selfupdate; do GOOS=windows go test -c -o /dev/null ./internal/$p/ || exit 1; done'   # компилирует Windows-тесты
bash .claude/heavy.sh go test ./internal/... ./tools/...
bash .claude/heavy.sh go test -race -tags devmock ./internal/...
bash .claude/heavy.sh golangci-lint run --new-from-rev=origin/dev ./...
bash .claude/heavy.sh golangci-lint run --new-from-rev=origin/dev --build-tags devmock ./internal/...
bash .claude/heavy.sh go run ./tools/lintbaseline
bash .claude/heavy.sh go run ./tools/lintbaseline -tags devmock
```

Агент прогоняет только пакеты, которые менял; полный `./internal/...`, `-race` и базис гоняет
один раз основная сессия или `typhon-verify` после слияния веток.

Проверка CI без мержа: ветка пушится в `dev` либо открывается PR в `dev` — `ci.yml` стартует
на push в `dev` и на pull request, но это только быстрые проверки (Windows-тесты, фронтенд).
Сборка под macOS, `-race` на macOS и полный линт на трёх ОС идут лишь по `workflow_dispatch`
с `full_checks=true` (`gh workflow run ci.yml --ref <ветка> -f full_checks=true`); без него
эти джобы пропускаются, и зелёный PR ничего не говорит о маке.

`GOOS=windows go build -tags devmock ./internal/devmock/` и `go build -tags production,devmock
./internal/devmock/` обязаны падать — это и есть гарантия, что мок не уедет в релиз. Тесты,
которые по смыслу проверяют только Windows (буквы дисков, реестр, COM), живут в
`*_windows_test.go`; всё остальное должно быть зелёным на macOS без `t.Skip`.

## macOS через CrossOver: `internal/wine`

Отдельно от `devmock` (который мокает Windows-подсистемы для разработки) есть настоящая
поддержка macOS: лаунчер собирается нативно, а windows-игры ставятся и запускаются через
установленный у пользователя CrossOver. Дизайн и план — в `docs/superpowers/` (каталог в
`.gitignore`, живёт только локально).

- `internal/wine` — единственное место, знающее про CrossOver. Умеет найти рантайм
  (`Detect`), завести бутыль (`Ensure`), запустить в нём команду (`Run`, `StartDetached`),
  перечислить и убить процессы (`Processes`, `AllProcesses`, `Kill`), прочитать записи
  удаления из `system.reg` и перевести путь между native и windows. Не импортирует
  `platform`, `install`, `library` и `procs` — зависимость строго в одну сторону.
- **Бутыль на игру** из шаблона `win10_64`, ключ — каталог установки. Учёт ведётся меткой
  `typhon-bottle.json` внутри самого бутыля, а не файлом в `<ConfigDir>`: так он не может
  разойтись с файловой системой, и `SaveRoots()` (функция без аргументов) обходится без
  протаскивания путей; `procs.List(ctx)` тоже не получает путей установок и находит бутыли сам.
- **Игра лежит вне бутыля**, в обычной папке библиотеки, и видна в нём через симлинк-диск в
  `dosdevices`. Букву выбираем из свободных (`t..x`, потом `l..s`) и принудительно
  возвращаем на папку игр при каждом `Ensure`: CrossOver при обновлении бутыля раздаёт
  буквы смонтированным томам и может занять нашу. Благодаря этому бутыль остаётся ~300 МБ,
  а модель путей библиотеки не меняется.
- **Общий бутыль со Steam — режим по умолчанию.** Игра, которой не запретили
  этого явно, запускается в пользовательском бутыле с windows Steam (имя —
  `wine.SharedBottleName()`: константа `Steam`, переопределяется
  `TYPHON_STEAM_BOTTLE`), а не в собственном: Steam API, оверлей и достижения
  работают только в одном префиксе со Steam. Признак — `Game.RequiresSteam
  *bool`, трёхзначный: `nil` — на усмотрение лаунчера (значит «в общий, если
  он есть»), явный `false` — единственный способ потребовать изоляции.
  Собственный бутыль остаётся запасным путём и работает как прежде.
  Общий бутыль **не наш**: `SharedBottle` его только находит, метку
  `typhon-bottle.json` в него не пишет, поэтому `List`/`Lookup`/`Remove` его
  не видят и удалить не могут. Игра внутрь `drive_c` не копируется — бутыль
  видит её через одну из своих букв (у CrossOver всегда есть хотя бы `z:` на
  корень ФС), причём буква выбирается самая длинная подходящая, ровно как это
  делает сам wine: домашние пути становятся `Y:\…`, а не `Z:\…`.
  Три места, где общий бутыль пришлось учесть отдельно, потому что все они
  ходили через `List()`: снимок процессов (`AllProcesses` — без этого цикл
  детекта закрывал сессию на первом же тике), корни сейвов
  (`platform.SaveRoots`) и записи удаления (`install.readUninstallEntries`).
- **«Стоп» в общем бутыле — не `wineserver -k`.** Он свалил бы вместе с игрой
  и Steam, и соседние игры того же префикса. `Manager.KillProcesses` гасит по
  pid только процессы, чей путь лежит внутри каталога этой установки; отмена
  установки (`Run` по отменённому ctx) ходит туда же через `stopBottle`.
- **Повышения прав на macOS нет**: UAC не существует, установщик запускается напрямую, весь
  протокол воркера, state- и cancel-файлов остаётся Windows-только.
- **Детект сессии** — разбор `ps -ax -o pid,lstart,command`: wine печатает виндовый путь exe
  и время старта, `procs_darwin.go` переводит путь обратно в native, поэтому `library` не
  меняется вообще. `StopGame` — `wineserver -k` по `WINEPREFIX` бутыля, что безопасно ровно
  потому, что бутыль на одну игру.
- **Теги**: новые файлы `//go:build darwin && !devmock`; у `*_other.go`, получивших
  darwin-аналог, добавлено `&& !darwin`. `uninstall_other.go` — `!windows && (!darwin ||
  devmock)`, потому что devmock на маке пользуется общей заглушкой. Общие помощники разведки
  лежат в `discovery_shared.go` с тегом `windows || (darwin && !devmock)`: в devmock-сборке
  они мёртвый код и роняют линтер.
- **Живые проверки** против настоящего CrossOver — `internal/wine/live_darwin_test.go`, по
  умолчанию пропускаются, включаются `TYPHON_WINE_LIVE=1 go test ./internal/wine/ -run
  Live`. Они заводят и сносят настоящий бутыль (~13 секунд, ~300 МБ) и уже нашли один
  реальный баг: `CombinedOutput` в `StartDetached` ждал закрытия пайпов, унаследованных
  игрой, то есть висел до выхода из игры.

- **Связка ключей.** Нативная сборка не стартовала вовсе, пока хранилище учётных данных
  было только под Windows и devmock: `credential_darwin.go` кладёт токен в Keychain через
  `/usr/bin/security`. Не через Security.framework сознательно — там ACL выписывается на
  конкретный бинарь, и каждая пересборка спрашивала бы у пользователя доступ. Секрет идёт
  в stdin (аргументы видны в `ps`) и кодируется base64. Живая проверка —
  `TYPHON_KEYCHAIN_LIVE=1`.
- **Живая установка** через настоящий бутыль — `internal/install/runner_darwin_live_test.go`,
  включается `TYPHON_WINE_LIVE=1` плюс `TYPHON_WINE_LIVE_INSTALLER` с путём до любого
  windows-установщика (проверялось на `innosetup-6.7.3.exe`).

- **Запуск не блокирует.** `cxstart` подменяет себя `winewrapper`, и тот остаётся прямым
  потомком лаунчера на всё время игры: и `CombinedOutput`, и `Run` в `StartDetached` вешали
  `PlayGame` вместе с мьютексом библиотеки. Запускаем и отпускаем, факт запуска
  подтверждает появление процесса в бутыле.
- **Свежий бутыль прогревается** (`Manager.Boot`, `wineboot -u`) в момент создания, то есть
  во время установки: первый запуск инициализирует префикс минутами, и ждать их по нажатию
  «Играть» нельзя.
- **Подготовка окружения — подменяемое поле сервиса** (`install.Service.prepareRuntime`,
  `library.Service.prepare`), а не прямой вызов. Первая версия звала настоящий CrossOver
  прямо из тестов и оставила после прогона 29 настоящих бутылей на 12 ГБ. Любой новый
  тестовый конструктор обязан её заглушить.

Проверено в живом приложении 2026-09-06 целиком, на настоящей игре: `bin/typhon.app`
стартует, «О программе» показывает `darwin/arm64`, macOS 26.6.2 и «Среда запуска игр —
CrossOver 26.3». Hollow Knight Silksong (10 ГБ, Unity, портативная сборка) скачана,
опознана как портативная, перенесена в папку игр, получила бутыль, запущена в 2560x1440,
сессия отслежена и остановлена кнопкой «Стоп» с записью времени игры.

**Трей на macOS работает**, но проверять его надо правильно. Перечисление окон через
`CGWindowListCopyWindowInfo` на macOS 26 статус-элементы **не показывает** — ни наши, ни
чужих приложений, — поэтому «окна в слое статус-бара нет» ничего не доказывает. Проверять
через Accessibility:

```bash
osascript -e 'tell application "System Events" to tell process "typhon" to get count of menu bars'
# 2 — статус-элемент есть; 1 — его нет
osascript -e 'tell application "System Events" to tell process "typhon" to click menu bar item 1 of menu bar 2'
```

Настоящая поломка была не в трее: с `Mac.ApplicationShouldTerminateAfterLastWindowClosed:
true` macOS завершала приложение по закрытию окна, и сворачивать было уже некуда. Опция
теперь равна `!MinimizeToTray`, а хук закрытия при отсутствии трея выходит явным `Quit()` —
иначе остался бы процесс без окна и без иконки.

Ещё про macOS 26: система ввела разрешение «Разрешить в строке меню» (Системные настройки →
Строка меню), и у некоторых приложений статус-элемент по умолчанию скрыт. У нас это не
проявилось, но если у пользователя иконки нет при живом трее — смотреть надо туда.

## Релиз под macOS

- Артефакт самообновления — **zip с бандлом**, а не dmg: образ пришлось бы монтировать
  через `hdiutil` и отмонтировать при любой ошибке. Пакуется `ditto`, а не `zip`: только он
  сохраняет права, симлинки и ресурсные вилки так, как macOS ждёт их обратно. Задача —
  `darwin:package:zip`.
- Бандл **только `arm64`**, интеловый слайс убран вместе с задачами `*:universal`. На
  универсальный бандл macOS 26 показывает окно «Support Ending for Intel-Based Apps»
  («includes a component that will not work with a future release of macOS») даже тогда,
  когда приложение стартует нативно; Tahoe — последняя macOS для интеловых маков, а в
  macOS 27 режут Rosetta, так что слайс пугал бы всех ради тех, кому и так не обновиться.
  Релизный job проверяет `lipo -archs` и падает, если в бандле снова появился `x86_64`.
  Манифест объявляет один маковский артефакт, под `arm64`. `Kind` артефакта — `bundle`,
  отдельно от `installer`: на Windows файлы кладёт установщик, на macOS их подменяет сам
  лаунчер.
- `Apply` распаковывает рядом с бандлом (переименование работает только внутри тома),
  отодвигает старый бандл и возвращает его, если новый не встал, и снимает карантин.
  Архив приходит из сети, поэтому пути в нём проверяются: и запись наружу, и симлинк,
  уводящий туда же.
- **Подписи не будет.** Сертификат Apple Developer стоит денег и покупать его не планируют,
  поэтому шаг подписи в релизе не появится, а инструкция в README рассказывает, как
  разрешить приложение вручную. Ad-hoc подпись (`codesign --sign -`) остаётся: локально
  запущенная сборка работает, блокируется только скачанная.
- Релиз собирает отдельный job `macos` («Build the macOS bundle») и кладёт бандл в артефакты
  сборки. Его забирает не windows-job, а третий, `release` («Sign and publish both
  platforms», `needs: [macos, windows]`, `.github/workflows/release.yml`): он скачивает оба
  артефакта, проверяет их, подписывает манифест и публикует релиз. Job `macos` входит в
  релизные теги v0.5.0…v0.9.0 (`git tag --contains 52d1c0f`), прогоны Release для
  v0.7.0…v0.9.0 зелёные (`gh run list --workflow release.yml`). Это доказывает, что
  пайплайн собирает и публикует манифест с двумя артефактами; как собранный бандл ведёт себя
  у пользователей на самообновлении, зелёный прогон не говорит.

Сделано с тех пор: ярлыки-`.app`, автозапуск через собственный LaunchAgent (не
`wails.Autostart`: SMAppService опознаёт приложение по подписи, которой у нас не будет),
трей, самообновление бандла, журнал совместимости игр и общая статистика по нему.

Не сделано: DMG и подпись Apple Developer.
