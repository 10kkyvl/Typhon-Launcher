package wine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	entries := parsePS(out)
	result := make([]Process, 0, len(entries))
	for _, entry := range entries {
		for _, b := range bottles {
			if p, ok := match(entry, b); ok {
				result = append(result, p)
				break
			}
		}
	}
	return result, nil
}

func match(entry psEntry, b Bottle) (Process, bool) {
	native, err := b.ToNative(entry.winPath)
	if err != nil || !underKey(native, b.Key) {
		return Process{}, false
	}
	return Process{PID: entry.pid, WinPath: entry.winPath, Path: native, CreatedAt: entry.createdAt}, true
}

// Kill валит бутыль целиком. Это безопасно ровно потому, что бутыль заведён
// под одну установку: чужого в нём нет.
func (m *Manager) Kill(b Bottle) error {
	//nolint:gosec // G204: путь до wineserver получен из Detect
	cmd := exec.Command(m.rt.WineServer, "-k")
	cmd.Env = append(os.Environ(), "WINEPREFIX="+b.Path, "CX_BOTTLE="+b.Name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("остановка бутыля %s: %w: %s", b.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
