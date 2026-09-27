//go:build darwin && !devmock

package install

// workerProcessAlive: повышенного воркера на macOS не бывает, поэтому его
// здесь никогда нет — ложь без ошибки, а не отказ, потому что вызывающие
// относятся к «не воркер» и «воркер мёртв» одинаково.
func workerProcessAlive(int) (bool, error) {
	return false, nil
}
