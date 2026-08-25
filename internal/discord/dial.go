package discord

import (
	"context"
	"io"
)

type dialAttempt struct {
	conn io.ReadWriteCloser
	err  error
}

// dialCancelable запускает attempt в горутине и ждёт первого из двух: результата
// или ctx.Done(): os.OpenFile на именованном канале не принимает контекст и
// может надолго заблокироваться (антивирус держит хэндл на
// \\.\pipe\discord-ipc-N), а ранний возврат из ServiceShutdown важнее, чем
// ожидание завершения уже начатого системного вызова. Ни горутина, ни хэндл при
// этом не текут: attempt рано или поздно вернётся сам, канал буферизован, а
// соединение, открывшееся уже после отмены, забирает и закрывает discardDial.
func dialCancelable(ctx context.Context, attempt func() (io.ReadWriteCloser, error)) (io.ReadWriteCloser, error) {
	resc := make(chan dialAttempt, 1)
	go func() {
		conn, err := attempt()
		resc <- dialAttempt{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		go discardDial(resc)
		return nil, ctx.Err()
	case r := <-resc:
		return r.conn, r.err
	}
}

func discardDial(resc <-chan dialAttempt) {
	if r := <-resc; r.conn != nil {
		closeConn(r.conn)
	}
}
