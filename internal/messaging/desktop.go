package messaging

import (
	"context"
	"encoding/binary"
	"html"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Desktop shows a small non-activating window independently of the main
// webview, including when the launcher is minimised to its tray.
// All window operations are performed outside mu: Wails invokes the UI thread
// synchronously, and its click handlers may in turn call back into Desktop.
type Desktop struct {
	ctx         context.Context
	mu          sync.Mutex
	app         *application.App
	main, popup *application.WebviewWindow
	owner, peer string
	serial      uint64
	timer       *time.Timer
	soundPath   string
	soundCancel context.CancelFunc
	wg          sync.WaitGroup
	closed      bool
	lastSound   time.Time
	unsub       []func()
	suppress    func() bool
}

//wails:ignore
func NewDesktop(ctx context.Context, app *application.App, main *application.WebviewWindow) *Desktop {
	d := &Desktop{ctx: ctx, app: app, main: main}
	d.popup = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "chat-notification", Title: "Typhon", Width: 360, Height: 124,
		Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.RGBA{},
		HTML:             popupHTML("Typhon", ""), AllowSimpleEventEmit: true,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
		Mac:     application.MacWindow{Backdrop: application.MacBackdropTransparent, WindowClass: application.MacWindowClassPanel, PanelPreferences: application.MacPanelPreferences{NonActivating: true, FloatingPanel: true, BecomesKeyOnlyIfNeeded: true}, CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary},
	})
	d.unsub = append(d.unsub, app.Event.On("chat:popup-open", func(e *application.CustomEvent) { d.open() }), app.Event.On("chat:popup-close", func(e *application.CustomEvent) { d.Clear() }))
	if f, err := os.CreateTemp("", "typhon-message-*.wav"); err == nil {
		_, writeErr := f.Write(messageTone())
		closeErr := f.Close()
		if writeErr == nil && closeErr == nil {
			d.soundPath = f.Name()
		} else {
			slog.Warn("prepare chat notification sound", "write_error", writeErr, "close_error", closeErr)
			removeSoundFile(f.Name())
		}
	}
	return d
}

const imageNotice = "Изображение"

func popupHTML(title, body string) string {
	// No user-controlled HTML, URL or event name is interpolated.
	return `<!doctype html><html><meta charset="utf-8"><style>*{box-sizing:border-box}body{position:relative;margin:0;padding:10px 12px 14px;transition:opacity 240ms ease;background:transparent;color:#edf2f7;font:14px -apple-system,Segoe UI,sans-serif;animation:fade 240ms ease}@keyframes fade{from{opacity:0}}@keyframes rise{from{transform:translateY(8px)}}button{font:inherit;color:inherit;cursor:pointer}#open{position:relative;display:flex;align-items:center;gap:12px;width:100%;height:100px;overflow:hidden;text-align:left;background:#141b24;border:1px solid #2a3846;border-radius:14px;padding:12px 40px 12px 18px;box-shadow:0 8px 20px rgba(0,0,0,.45),0 1px 3px rgba(0,0,0,.4);animation:rise 240ms ease}#open:hover{background:#18212c}#open::before{content:"";position:absolute;left:0;top:0;bottom:0;width:4px;background:linear-gradient(180deg,#76b9ff,#4a7fd6)}.av{flex:none;display:grid;place-items:center;width:40px;height:40px;border-radius:50%;background:linear-gradient(135deg,#76b9ff,#4a7fd6);overflow:hidden}.av i{display:block;font:normal 0/1 -apple-system,Segoe UI,sans-serif;white-space:nowrap}.av i::first-letter{font-size:17px;font-weight:700;color:#0d1520;text-transform:uppercase}.txt{flex:1;min-width:0}.head{display:flex;align-items:baseline;gap:8px;margin-bottom:3px}b{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}small{flex:none;font-size:11px;color:#8c9cac}p{margin:0;font-size:13px;line-height:1.35;color:#becbda;overflow:hidden;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow-wrap:anywhere}#close{position:absolute;right:20px;top:18px;width:22px;height:22px;padding:0;border:0;border-radius:50%;background:none;font-size:16px;line-height:22px;color:#8c9cac}#close:hover{background:#26323f;color:#edf2f7}button:focus-visible{outline:2px solid #76b9ff;outline-offset:-3px}</style><button id="open" onclick="window._wails.invoke('wails:event:emit:chat:popup-open')"><span class="av" aria-hidden="true"><i>` + html.EscapeString(title) + `</i></span><span class="txt"><span class="head"><b>` + html.EscapeString(title) + `</b><small>Новое сообщение</small></span><p>` + html.EscapeString(body) + `</p></span></button><button id="close" aria-label="Close" onclick="window._wails.invoke('wails:event:emit:chat:popup-close')">×</button></html>`
}

// The popup chrome is Russian-only, so an image-only message gets the same language.
func popupBody(body string) string {
	if strings.TrimSpace(body) == "" {
		return imageNotice
	}
	return body
}

func (d *Desktop) Notify(owner, peer, title, body string) bool {
	return application.InvokeSyncWithResult(func() bool { return d.notifyOnUI(owner, peer, title, body) })
}

func (d *Desktop) notifyOnUI(owner, peer, title, body string) bool {
	body = popupBody(body)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return false
	}
	d.playLocked()
	d.mu.Unlock()
	if d.suppressed() {
		return false
	}
	if d.main.IsFocused() && d.main.IsVisible() && !d.main.IsMinimised() {
		return false
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return false
	}
	d.owner = owner
	d.peer = peer
	d.serial++
	serial := d.serial
	if d.timer != nil {
		d.timer.Stop()
	}
	d.mu.Unlock()
	// Initialise the hidden window before Show; its first call may only create
	// the native handle, which Windows also needs for the no-activation style.
	d.popup.Run()
	d.popup.SetHTML(popupHTML(title, body))
	if screen, err := d.main.GetScreen(); err == nil && screen != nil {
		a := screen.WorkArea
		d.popup.SetPosition(a.X+max(0, a.Width-376), a.Y+max(0, a.Height-128))
	}
	showWithoutActivation(d.popup)
	d.scheduleOnUI(serial, 4*time.Second, func() {
		d.popup.ExecJS("document.body.style.opacity='0'; document.body.style.pointerEvents='none'")
		d.scheduleOnUI(serial, 240*time.Millisecond, d.clearOnUI)
	})
	return true
}

//wails:ignore
func (d *Desktop) SetSuppress(fn func() bool) {
	d.mu.Lock()
	d.suppress = fn
	d.mu.Unlock()
}

func (d *Desktop) suppressed() bool {
	d.mu.Lock()
	fn := d.suppress
	d.mu.Unlock()
	return fn != nil && fn()
}

// Both dismissal stages belong to the current notification. A replacement
// cancels the old fade/hide timer, including callbacks already queued on the UI.
func (d *Desktop) scheduleOnUI(serial uint64, delay time.Duration, action func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.serial != serial {
		return
	}
	d.timer = time.AfterFunc(delay, func() {
		application.InvokeAsync(func() {
			d.mu.Lock()
			current := !d.closed && d.serial == serial
			d.mu.Unlock()
			if current {
				action()
			}
		})
	})
}
func (d *Desktop) open() {
	application.InvokeSync(d.openOnUI)
}

func (d *Desktop) openOnUI() {
	d.mu.Lock()
	owner, peer := d.owner, d.peer
	d.mu.Unlock()
	d.clearOnUI()
	if owner == "" || peer == "" {
		return
	}
	d.main.Restore()
	d.main.Show()
	d.main.Focus()
	d.app.Event.Emit("chat:open", OpenEvent{OwnerID: owner, PeerID: peer})
}
func (d *Desktop) Clear() {
	application.InvokeSync(d.clearOnUI)
}

func (d *Desktop) clearOnUI() {
	d.mu.Lock()
	d.owner = ""
	d.peer = ""
	d.serial++
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	closed := d.closed
	d.mu.Unlock()
	if !closed {
		d.popup.Hide()
	}
}
func (d *Desktop) Close() {
	d.mu.Lock()
	d.closed = true
	d.owner, d.peer = "", ""
	if d.timer != nil {
		d.timer.Stop()
	}
	if d.soundCancel != nil {
		d.soundCancel()
	}
	d.mu.Unlock()
	for _, unsub := range d.unsub {
		unsub()
	}
	d.wg.Wait()
	if d.soundPath != "" {
		removeSoundFile(d.soundPath)
	}
}
func (d *Desktop) playLocked() {
	// Burst messages share a chime rather than layering loud sounds.
	if d.soundPath == "" || time.Since(d.lastSound) < 1500*time.Millisecond {
		return
	}
	d.lastSound = time.Now()
	if d.soundCancel != nil {
		d.soundCancel()
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.soundCancel = cancel
	d.wg.Add(1)
	go func() { defer d.wg.Done(); defer cancel(); playTone(ctx, d.soundPath) }()
}

// An original, short two-note chime; no external audio asset or codec required.
func messageTone() []byte {
	const rate = 22050
	const count = rate * 42 / 100
	out := make([]byte, 44+count*2)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], 36+count*2)
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], 1)
	binary.LittleEndian.PutUint32(out[24:], rate)
	binary.LittleEndian.PutUint32(out[28:], rate*2)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], count*2)
	for i := 0; i < count; i++ {
		t := float64(i) / rate
		sample := 0.0
		for j, f := range []float64{329.63, 440} {
			x := t - float64(j)*0.11
			if x >= 0 {
				// Keep the soft attack while raising the user-selected volume.
				sample += 0.22 * math.Min(1, x/0.025) * math.Exp(-x*12) * math.Sin(2*math.Pi*f*x)
			}
		}
		binary.LittleEndian.PutUint16(out[44+i*2:], uint16(int32(sample*32767)&0xffff))
	}
	return out
}

func removeSoundFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove chat notification sound", "error", err)
	}
}
