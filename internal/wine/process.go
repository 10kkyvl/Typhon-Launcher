package wine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Process — один процесс, работающий внутри бутыля.
type Process struct {
	PID       int
	WinPath   string
	Path      string
	CreatedAt time.Time
}

type psEntry struct {
	pid       int
	winPath   string
	createdAt time.Time
}

// psLine разбирает строку `ps -ax -o pid,lstart,command`: pid, время старта в
// формате lstart, затем команда. Нас интересуют только строки, где команда
// начинается с буквы диска — так wine печатает windows-программы, и это
// единственный признак, отличающий их от нативных процессов.
var psLine = regexp.MustCompile(`^\s*(\d+)\s+(\w{3}\s+\w{3}\s+\d+\s+\d+:\d+:\d+\s+\d{4})\s+([A-Za-z]:\\.*)$`)

// psExeSuffix отрезает аргументы: путь до exe кончается на .exe, а всё после —
// параметры запуска. Пробелы внутри самого пути при этом сохраняются.
var psExeSuffix = regexp.MustCompile(`(?i)^(.*?\.exe)(\s.*)?$`)

const lstartLayout = "Mon Jan _2 15:04:05 2006"

func parsePS(out string) []psEntry {
	entries := make([]psEntry, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		match := psLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		pid, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		started, err := time.ParseInLocation(lstartLayout, normalizeSpaces(match[2]), time.Local)
		if err != nil {
			continue
		}
		path := strings.TrimSpace(match[3])
		if exe := psExeSuffix.FindStringSubmatch(path); exe != nil {
			path = exe[1]
		}
		entries = append(entries, psEntry{pid: pid, winPath: path, createdAt: started})
	}
	return entries
}

// normalizeSpaces сводит двойной пробел lstart для однозначных дней к
// одинарному, который понимает layout с _2.
func normalizeSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (m *Manager) processList() (string, error) {
	if m.psOutput != nil {
		return m.psOutput()
	}
	out, err := exec.Command("ps", "-ax", "-o", "pid,lstart,command").Output()
	if err != nil {
		return "", fmt.Errorf("перечисление процессов: %w", err)
	}
	return string(out), nil
}

func (m *Manager) processEnvironment(pid int) (string, error) {
	if m.processEnv != nil {
		return m.processEnv(pid)
	}
	//nolint:gosec // G204: pid is converted to base-10 digits before being passed directly to ps
	out, err := exec.Command("ps", "e", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return "", fmt.Errorf("ps %d: %w", pid, err)
	}
	return string(out), nil
}

var environmentAssignment = regexp.MustCompile(`\s+[A-Za-z_][A-Za-z0-9_]*=`)

func processRunsInBottle(environment, bottle string) bool {
	if bottle == "" {
		return false
	}
	value, ok := processEnvironmentValue(environment, "CX_BOTTLE")
	return ok && value == bottle
}

func processEnvironmentValue(environment, key string) (string, bool) {
	text := " " + environment
	marker := " " + key + "="
	start := strings.LastIndex(text, marker)
	if start < 0 {
		return "", false
	}
	value := text[start+len(marker):]
	if next := environmentAssignment.FindStringIndex(value); next != nil {
		value = value[:next[0]]
	}
	return strings.TrimSpace(value), true
}

// Processes отбирает процессы, чей путь после перевода в native лежит внутри
// каталога установки этого бутыля. Буква сама по себе не признак: разные
// бутыли могут независимо выбрать одну и ту же свободную букву.
func (m *Manager) Processes(ctx context.Context, b Bottle) ([]Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := m.processList()
	if err != nil {
		return nil, err
	}
	result := make([]Process, 0, 4)
	for _, entry := range parsePS(out) {
		if p, ok := match(entry, b); ok {
			result = append(result, p)
		}
	}
	return result, nil
}

// AllProcesses перечисляет процессы всех наших бутылей: цикл детекта игр
// спрашивает про систему целиком, а не про конкретную игру.
//
// Общий бутыль (тот, где стоит Steam) добавляется сюда отдельной веткой, и
// это не роскошь: без неё игра, запущенная в нём, не попадает ни в один
// снимок, а цикл детекта закрывает её сессию на первом же тике — то есть
// через секунду после старта.
func (m *Manager) AllProcesses(ctx context.Context) ([]Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bottles, err := m.List()
	if err != nil {
		return nil, err
	}
	out, err := m.processList()
	if err != nil {
		return nil, err
	}
	shared, err := m.sharedDrives()
	if err != nil {
		return nil, err
	}
	entries := parsePS(out)
	result := make([]Process, 0, len(entries))
	for _, entry := range entries {
		if p, ok := matchAny(entry, bottles); ok {
			result = append(result, p)
			continue
		}
		// Ключа установки у общего бутыля здесь нет: какие каталоги
		// принадлежат играм, знает библиотека, и она же сверит путь. Наше
		// дело — перевести путь в native, чтобы ей было что сверять.
		//
		// Диск c: пропускается: за ним внутренности самого бутыля — Steam,
		// системные службы, explorer, — а игры в общем бутыле лежат
		// снаружи. Без этого каждый снимок приносил бы десяток чужих строк.
		if strings.HasPrefix(strings.ToLower(entry.winPath), `c:\`) {
			continue
		}
		if native, ok := shared.toNative(entry.winPath); ok {
			result = append(result, Process{
				PID: entry.pid, WinPath: entry.winPath, Path: native, CreatedAt: entry.createdAt,
			})
		}
	}
	return result, nil
}

// sharedDrives отдаёт буквы общего бутыля. Отсутствие бутыля не ошибка: на
// машине без Steam общих игр просто нет.
func (m *Manager) sharedDrives() (driveMap, error) {
	if m.BottlesDir == "" {
		return nil, nil
	}
	driveTable, err := drives(filepath.Join(m.BottlesDir, SharedBottleName()))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return driveTable, err
}

func matchAny(entry psEntry, bottles []Bottle) (Process, bool) {
	for _, b := range bottles {
		if p, ok := match(entry, b); ok {
			return p, true
		}
	}
	return Process{}, false
}

func match(entry psEntry, b Bottle) (Process, bool) {
	native, err := b.ToNative(entry.winPath)
	if err != nil || !underKey(native, b.Key) {
		return Process{}, false
	}
	return Process{PID: entry.pid, WinPath: entry.winPath, Path: native, CreatedAt: entry.createdAt}, true
}

// defaultKillTimeout ограничивает Kill(b Bottle) error — сигнатуру без ctx,
// оставленную ради вызывающих вне пакета (internal/library, а после недавней
// правки и internal/wine/run.go), которым сигнатуру менять нельзя. Без этого
// таймаута подвисший wineserver (известный класс проблем wine) вешал вызов
// навсегда: горутина Wails-биндинга не возвращалась, и «Стоп» переставал
// работать для этой игры до перезапуска лаунчера.
const defaultKillTimeout = 10 * time.Second

// killReapGrace — сколько ждём cmd.Wait() уже после SIGKILL. Процесса к
// этому моменту нет, ждём только закрытия унаследованного пайпа, и ждать
// его дольше нечего: вызывающему всё равно вернётся ошибка таймаута.
const killReapGrace = 200 * time.Millisecond

// Kill валит бутыль целиком. Это безопасно ровно потому, что бутыль заведён
// под одну установку: чужого в нём нет.
//
// Сигнатура намеренно без ctx: её меняют вызывающие вне этого пакета
// (internal/library, run.go), а их трогать нельзя. Таймаут собран вручную, а
// не через context.WithTimeout(context.Background(), ...): последнее завело
// бы внутри пакета корневой контекст без родителя, а contextcheck справедливо
// считает это поводом требовать проброса ctx от вызывающих — которым, в
// отличие от KillContext, взять ctx неоткуда по своей сигнатуре.
func (m *Manager) Kill(b Bottle) error {
	timeout := m.killTimeout
	if timeout <= 0 {
		timeout = defaultKillTimeout
	}
	//nolint:gosec // G204: путь до wineserver получен из Detect
	cmd := exec.Command(m.rt.WineServer, "-k")
	cmd.Env = append(os.Environ(), "WINEPREFIX="+b.Path, "CX_BOTTLE="+b.Name)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("остановка бутыля %s: %w", b.Name, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("остановка бутыля %s: %w: %s", b.Name, err, strings.TrimSpace(out.String()))
		}
		return nil
	case <-timer.C:
		if killErr := cmd.Process.Kill(); killErr != nil {
			return fmt.Errorf("остановка бутыля %s: не уложился в %s, и не удалось прибить wineserver: %w",
				b.Name, timeout, killErr)
		}
		// Дожидаемся Wait ограниченно, иначе таймаут выше не соблюдается.
		// Stdout/Stderr здесь — bytes.Buffer, то есть os/exec отдаёт процессу
		// пайп и копирует из него в отдельной горутине; Wait возвращается
		// только после того, как пайп закрыли все, кто его унаследовал.
		// Зависший wineserver — это ровно тот случай, когда у него есть живые
		// потомки: убитый родитель пайп не закрывает, и Wait ждёт их, а не
		// его. Горутина с Wait завершится сама, когда потомки уйдут; out в
		// этой ветке никто не читает, так что гонки за буфер нет.
		select {
		case <-done:
		case <-time.After(killReapGrace):
		}
		return fmt.Errorf("остановка бутыля %s: wineserver не уложился в %s", b.Name, timeout)
	}
}

// KillContext — как Kill, но с настоящей отменой: вызывающий с собственным
// ctx не обязан ждать таймаут по умолчанию.
func (m *Manager) KillContext(ctx context.Context, b Bottle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	//nolint:gosec // G204: путь до wineserver получен из Detect
	cmd := exec.CommandContext(ctx, m.rt.WineServer, "-k")
	cmd.Env = append(os.Environ(), "WINEPREFIX="+b.Path, "CX_BOTTLE="+b.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("остановка бутыля %s: %w", b.Name, ctxErr)
		}
		return fmt.Errorf("остановка бутыля %s: %w: %s", b.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
