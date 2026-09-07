---
name: lint-baseline
description: Use when golangci-lint findings were fixed or added and `.github/lint-baseline.txt` must be re-measured or lowered, when `go run ./tools/lintbaseline` or the CI "lint baseline" step fails with a count above baseline, or when a Windows/Linux lint number is needed from a Mac.
---

# Базис линта: как мерить честно

Базис — число находок `golangci-lint run ./...` на чистом дереве, по ключу `GOOS` или
`GOOS+теги`, в `.github/lint-baseline.txt`. Число может только уменьшаться. CI меряет
на Linux, macOS (плюс `devmock`) и Windows и падает, если стало больше.

## Что можно измерить с мака

| Ключ | С мака | Как |
|---|---|---|
| `darwin`, `darwin+devmock` | да | нативно |
| `windows` | да, честно | `GOOS=windows`: анализ типизируется, Windows-only файлы (`*_windows.go`) входят в выдачу |
| `linux`, `linux+devmock` | нет | wails тянет GTK через cgo, прогон падает в одну находку `typecheck` — это не счёт |

Ключи `linux*` берутся только из CI: workflow `CI`, джоб «Format, vet, lint, cross-build»,
шаги «Lint baseline (linux)» и «Lint baseline (linux, devmock tag)» — в логе строка
`count=N`. Посмотреть: `gh run list --workflow ci.yml --branch <ветка>` и
`gh run view <id> --log | grep 'lint findings'`. Записывать под них цифру с мака
нельзя: она занизит планку и уронит CI.

## Процедура

```bash
export PATH="$HOME/go/bin:$PATH"
W=/tmp/typhon-lint-wt                      # фиксированный путь: cwd между вызовами Bash не живёт
git worktree add -q --detach "$W" HEAD     # чистое дерево: untracked-файлы меняют цифру
cd "$W" && go build -o /tmp/lb ./tools/lintbaseline
cd "$W" && /tmp/lb                                    # darwin
cd "$W" && /tmp/lb -tags devmock                      # darwin+devmock
cd "$W" && GOOS=windows /tmp/lb -goos windows         # windows; -goos обязателен, иначе запишется в ключ darwin
```

Все ключи совпали — файл не трогать, `-update` не нужен.

Тул сравнивает с базисом и печатает счёт; при превышении падает. Понизить — те же команды
с `-update`, файл в воркри потом скопировать в рабочее дерево (или сделать замер в рабочем
дереве после коммита, когда `git status --porcelain` пуст). Ключ `-goos` нужен, потому
что тул берёт `runtime.GOOS`, а под `GOOS=windows` бинарь всё равно маковский. Воркри убрать:
`git worktree remove --force "$W"`.

Что-то изменилось под Windows — дополнительно:

```bash
GOOS=windows golangci-lint run --new-from-rev=origin/dev ./...
for p in $(go list ./internal/...); do GOOS=windows go test -c -o /dev/null $p; done   # чеклист компилирует только install и selfupdate
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
