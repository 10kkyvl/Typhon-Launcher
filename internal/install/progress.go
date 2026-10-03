package install

import "time"

const progressInterval = 200 * time.Millisecond

type reporter struct {
	fn      func(Progress)
	total   int64
	done    int64
	current string
	last    time.Time
}

func newReporter(fn func(Progress), total int64) *reporter {
	return &reporter{fn: fn, total: total}
}

func (r *reporter) setFile(name string) {
	r.current = name
	r.tick()
}

func (r *reporter) add(n int64) {
	r.done += n
	r.tick()
}

func (r *reporter) restart() {
	r.done = 0
	r.current = ""
	r.flush()
}

func (r *reporter) setPercent(pct int) {
	pct = min(max(pct, 0), 100)
	done := r.total * int64(pct) / 100
	if done <= r.done {
		return
	}
	r.done = done
	r.tick()
}

func (r *reporter) tick() {
	if r.fn == nil {
		return
	}
	now := time.Now()
	if !r.last.IsZero() && now.Sub(r.last) < progressInterval {
		return
	}
	r.last = now
	r.emit()
}

func (r *reporter) flush() {
	if r.fn == nil {
		return
	}
	r.last = time.Now()
	r.emit()
}

func (r *reporter) emit() {
	r.fn(Progress{BytesDone: r.done, BytesTotal: r.total, CurrentFile: r.current})
}
