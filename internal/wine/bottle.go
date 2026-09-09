package wine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/storage"
)

// markerName — файл, по которому бутыль опознаётся как наш. Учёт бутылей
// выведен из файловой системы, а не из отдельного файла состояния: так он не
// может разойтись с реальностью, переживает потерю конфига, и пользователь
// видит рядом с бутылем, чей он.
const markerName = "typhon-bottle.json"

// Bottle — один бутыль CrossOver, отданный под одну установку.
type Bottle struct {
	Key   string // канонический путь каталога установки
	Name  string // имя бутыля в CrossOver
	Path  string // каталог самого бутыля
	Drive string // буква, под которой в бутыле видна папка игр
	Games string // native-путь папки игр

	// Shared — бутыль общий и не наш: в нём живёт windows Steam, рядом с ним
	// стоят другие игры, и валить его целиком нельзя. Метка на диске этого
	// поля не хранит: общий бутыль пользовательский, метки в нём нет и быть
	// не должно, поэтому readMarker всегда отдаёт Shared=false.
	Shared bool
}

type marker struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Drive string `json:"drive"`
	Games string `json:"games"`
}

func writeMarker(bottlePath string, b Bottle) error {
	data, err := json.MarshalIndent(marker{Key: b.Key, Name: b.Name, Drive: b.Drive, Games: b.Games}, "", "  ")
	if err != nil {
		return fmt.Errorf("метка бутыля: %w", err)
	}
	return storage.WriteAtomic(filepath.Join(bottlePath, markerName), append(data, '\n'))
}

// readMarker не отличает отсутствие метки от битой: и то и другое значит
// «этот бутыль не наш», а вызывающему в обоих случаях делать одно и то же.
func readMarker(bottlePath string) (Bottle, bool) {
	data, err := os.ReadFile(filepath.Join(bottlePath, markerName))
	if err != nil {
		return Bottle{}, false
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Bottle{}, false
	}
	if m.Key == "" || m.Drive == "" || m.Games == "" {
		return Bottle{}, false
	}
	return Bottle{Key: m.Key, Name: m.Name, Path: bottlePath, Drive: m.Drive, Games: m.Games}, true
}

// bottleName делает имя, которое человек узнает в UI CrossOver, но которое
// при этом заведомо безопасно как имя каталога: хвост хеша разводит игры с
// одинаковыми первыми буквами и снимает вопрос экранирования.
func bottleName(destDir string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, filepath.Base(destDir))
	safe = strings.Trim(safe, "-")
	if len(safe) > 32 {
		safe = strings.Trim(safe[:32], "-")
	}
	sum := sha256.Sum256([]byte(destDir))
	suffix := hex.EncodeToString(sum[:4])
	if safe == "" {
		return "Typhon-" + suffix
	}
	return "Typhon-" + safe + "-" + suffix
}

// ToWindows переводит путь внутри папки игр в путь на букве бутыля.
func (b Bottle) ToWindows(native string) (string, error) {
	rel, err := filepath.Rel(b.Games, native)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		table, mapErr := drives(b.Path)
		if mapErr != nil {
			return "", mapErr
		}
		letter, base, ok := table.drive(native)
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrNoDriveForPath, native)
		}
		rel, err = filepath.Rel(base, native)
		if err != nil {
			return "", err
		}
		return strings.ToUpper(letter) + `:\` + strings.ReplaceAll(rel, "/", `\`), nil
	}
	return strings.ToUpper(b.Drive) + `:\` + strings.ReplaceAll(rel, "/", `\`), nil
}

// ToNative — обратный перевод. Пути на других буквах не наши: их вызывающий
// обязан отличать от своих, а не молча принимать.
//
// У общего бутыля буква одна, а путей много: игра лежит на своей, её записи
// в реестре — на C:, сейвы — на третьей. Поэтому там перевод идёт по всей
// таблице dosdevices, как это делает сам wine.
func (b Bottle) ToNative(win string) (string, error) {
	if b.Shared {
		driveTable, err := drives(b.Path)
		if err != nil {
			return "", err
		}
		native, ok := driveTable.toNative(win)
		if !ok {
			return "", fmt.Errorf("путь %s не выражается через диски бутыля %s", win, b.Name)
		}
		return native, nil
	}
	prefix := strings.ToUpper(b.Drive) + `:\`
	if !strings.HasPrefix(strings.ToUpper(win), prefix) {
		return "", fmt.Errorf("путь %s не на диске %s", win, prefix)
	}
	rel := strings.ReplaceAll(win[len(prefix):], `\`, "/")
	return filepath.Join(b.Games, rel), nil
}
