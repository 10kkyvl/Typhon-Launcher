//go:build !windows && !devmock

package install

// workerProcessAlive: воркер существует только на Windows, поэтому здесь его
// никогда нет — ложь без ошибки, а не отказ, потому что вызывающие относятся
// к "не воркер" и "воркер мёртв" одинаково.
func workerProcessAlive(int) (bool, error) {
	return false, nil
}
