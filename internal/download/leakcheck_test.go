package download

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMain проваливает пакет, если после всех тестов остались фоновые
// горутины базы готовности кусков. newManagerAt открывает её вместе с
// менеджером, а останавливает только Close, поэтому тест, создавший менеджер
// и не закрывший его, оставлял горутину жить до конца бинарника. Такая утечка
// не видна ни одному отдельному тесту — она проявляется гонкой в чужом тесте
// через много файлов, как и случилось на CI, — поэтому проверка стоит здесь,
// а не внутри теста.
func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if leaked := waitForCompletionLoopsToStop(2 * time.Second); leaked != "" {
			fmt.Fprintf(os.Stderr, "\nбаза готовности кусков осталась открытой после тестов:\n%s\n", leaked)
			code = 1
		}
	}
	os.Exit(code)
}

// waitForCompletionLoopsToStop возвращает стеки оставшихся flushLoop или
// пустую строку. Ожидание нужно потому, что Close отпускает вызывающего по
// закрытию done, а сама горутина в этот момент ещё доматывает свои defer:
// без ожидания проверка ловила бы этот хвост как утечку.
func waitForCompletionLoopsToStop(within time.Duration) string {
	deadline := time.Now().Add(within)
	for {
		stacks := completionLoopStacks()
		if len(stacks) == 0 {
			return ""
		}
		if time.Now().After(deadline) {
			return strings.Join(stacks, "\n")
		}
		runtime.Gosched()
	}
}

func completionLoopStacks() []string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "fileCompletion).flushLoop") {
			out = append(out, g)
		}
	}
	return out
}
