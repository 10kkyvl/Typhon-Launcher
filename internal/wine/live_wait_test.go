package wine

import "time"

// waitABit вынесен отдельно: живой проверке нужен именно сон между опросами
// внешней таблицы процессов, синхронизировать тут нечего.
func waitABit() { time.Sleep(500 * time.Millisecond) }
