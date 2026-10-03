//go:build windows

package install

import (
	"os/exec"
	"testing"
)

// plantLink ставит junction: его создаёт и пользователь без прав
// администратора, поэтому именно им подменяют каталог в общей папке.
func plantLink(t *testing.T, link, target string) {
	t.Helper()
	//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s %s: %v (%s)", link, target, err, out)
	}
}
