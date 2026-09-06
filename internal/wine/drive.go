package wine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoFreeDrive — все буквы, которые нам разрешено занимать, уже заняты.
var ErrNoFreeDrive = errors.New("wine: в бутыле нет свободной буквы диска")

// driveOrder — порядок выбора буквы. Сначала хвост алфавита: тома CrossOver
// раздаёт с начала, поэтому там столкновение вероятнее. c, y и z не наши:
// c — системный диск бутыля, y — домашний каталог, z — корень файловой
// системы, и всё это раздаёт сам CrossOver.
const driveOrder = "tuvwxlmnopqrs"

func freeDrive(dosdevices string) (string, error) {
	for _, r := range driveOrder {
		letter := string(r)
		if _, err := os.Lstat(filepath.Join(dosdevices, letter+":")); errors.Is(err, os.ErrNotExist) {
			return letter, nil
		}
	}
	return "", ErrNoFreeDrive
}

// ensureDrive держит букву нацеленной на папку игр. Симлинк переписывается
// безусловно: CrossOver при обновлении бутыля раздаёт буквы смонтированным
// томам и может занять нашу, а путь вида T:\Game\game.exe уже записан в
// реестр бутыля и в конфиги игры — протухнуть он не должен. Отобранный у
// тома символ не теряется: тот же том виден через z:.
func ensureDrive(bottlePath, letter, games string) error {
	link := filepath.Join(bottlePath, "dosdevices", letter+":")
	target, err := os.Readlink(link)
	if err == nil && target == games {
		return nil
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("снятие буквы %s: %w", letter, err)
	}
	if err := os.Symlink(games, link); err != nil {
		return fmt.Errorf("буква %s на %s: %w", letter, games, err)
	}
	return nil
}
