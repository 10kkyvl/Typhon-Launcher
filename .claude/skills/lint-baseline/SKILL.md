---
name: lint-baseline
description: Use when golangci-lint findings were fixed or added and `.github/lint-baseline.txt` must be re-measured or lowered, when `go run ./tools/lintbaseline` or the CI "Check platform baselines" step fails with a count above baseline, or when a Windows/Linux lint number is needed from a Mac.
---

# Базис линта: как мерить честно

Базис — число находок `golangci-lint run ./...` на чистом дереве, по ключу `GOOS` или
`GOOS+теги`, в `.github/lint-baseline.txt`. Число может только уменьшаться. Актуальные
цифры лежат в самом файле, в скилле и отчётах их не повторять — они устаревают с каждым
коммитом. `.golangci.yml` в git отслеживается; каталог `docs/` в `.gitignore`, поэтому
старые замеры вроде `docs/lint-baseline-v0.3.0.txt` есть только на машине, где их сняли.

В CI базис проверяет джоб `lint` («Full lint (ubuntu-latest | macos-latest | windows-latest)»,
шаг «Check platform baselines», он зовёт `bash scripts/check-local.sh lint`). Он идёт
**только** по `workflow_dispatch` с `full_checks=true`: на push в `dev` и на PR линт-джоба
нет, поэтому зелёный быстрый CI про базис ничего не говорит. На каждой ОС джоб меряет
свои ключи (Linux — `linux`, `linux+devmock` и `windows`; macOS — `darwin`, `darwin+devmock`
и `windows`; Windows — `windows`), на Linux дополнительно гоняет `--new-from-rev` и падает,
если число выросло.

## Что можно измерить с мака

| Ключ | С мака | Как |
|---|---|---|
| `darwin`, `darwin+devmock` | да | нативно |
| `windows` | да, честно | `GOOS=windows`: анализ типизируется, Windows-only файлы (`*_windows.go`) входят в выдачу |
| `linux`, `linux+devmock` | нет | wails тянет GTK через cgo, прогон падает в одну находку `typecheck` — это не счёт |

Ключи `linux*` берутся только из CI: workflow `CI`, джоб `lint` на `ubuntu-latest`, шаг
«Check platform baselines» — в логе строки `lint findings …` с `count=N` по каждому ключу.
Запустить: ветка должна быть на GitHub, затем
`gh workflow run ci.yml --ref <ветка> -f full_checks=true` (входной параметр `lint_base` —
ревизия для проверки новых находок на Linux, по умолчанию `origin/main`). Посмотреть:
`gh run list --workflow ci.yml --branch <ветка>` и
`gh run view <id> --job <id джоба lint ubuntu> --log | grep 'lint findings'`. Записывать под
эти ключи цифру с мака нельзя: она занизит планку и уронит CI.

## Процедура

Каждая тяжёлая команда идёт через очередь, отдельным вызовом `bash .claude/heavy.sh …`
(`golangci-lint` внутри тула — тоже тяжёлый). `heavy.sh` запускается из корня основного
репозитория, а в воркри команда переходит уже внутри `sh -c`: замок общий на все воркри.

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
W=/tmp/typhon-lint-wt                      # фиксированный путь: cwd между вызовами Bash не живёт
git worktree add -q --detach "$W" HEAD     # чистое дерево: untracked-файлы меняют цифру
bash .claude/heavy.sh sh -c 'cd /tmp/typhon-lint-wt && go build -o /tmp/lb ./tools/lintbaseline'
bash .claude/heavy.sh sh -c 'cd /tmp/typhon-lint-wt && /tmp/lb'                               # darwin
bash .claude/heavy.sh sh -c 'cd /tmp/typhon-lint-wt && /tmp/lb -tags devmock'                 # darwin+devmock
bash .claude/heavy.sh sh -c 'cd /tmp/typhon-lint-wt && GOOS=windows /tmp/lb -goos windows'    # windows; -goos обязателен, иначе запишется в ключ darwin
```

Все ключи совпали — файл не трогать, `-update` не нужен.

Тул сравнивает с базисом и печатает счёт; при превышении падает. Понизить — те же команды
с `-update`, файл в воркри потом скопировать в рабочее дерево (или сделать замер в рабочем
дереве после коммита, когда `git status --porcelain` пуст). Ключ `-goos` нужен, потому
что тул берёт `runtime.GOOS`, а под `GOOS=windows` бинарь всё равно маковский. Воркри убрать:
`git worktree remove --force "$W"`.

Что-то изменилось под Windows — дополнительно (тоже через очередь, по вызову на команду):

```bash
bash .claude/heavy.sh GOOS=windows golangci-lint run --new-from-rev=origin/dev ./...
bash .claude/heavy.sh sh -c 'for p in $(go list ./internal/...); do GOOS=windows go test -c -o /dev/null $p || exit 1; done'   # чеклист компилирует только install и selfupdate
```

## Закончить

- `git worktree remove "$W"`.
- Понижение базиса — в том же коммите, что и починка, а не отдельным «chore».
- В отчёте: три числа с мака и откуда взяты `linux*`, если их трогал (ссылка на CI-run).
- Число выросло — чинить код. Не `//nolint`, не `.golangci.yml`, не сужение скоупа.

## Частые ошибки

- «Windows с мака не измерить, ждём CI» — измерить можно, см. таблицу.
- Замер в рабочем дереве с untracked-файлами: цифра другая, базис врёт.
- `-update` без `-goos windows` под `GOOS=windows` записывает число в ключ `darwin`.
- Записал `linux` по маковскому прогону, CI упал с «count above baseline».
- Ждёшь линт-джоб от обычного push или PR: он стартует только по `workflow_dispatch` с `full_checks=true`.
