package messaging

import (
	"context"
	"encoding/binary"
	"html"
	"math"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Desktop shows a small non-activating window independently of the main
// webview, including when the launcher is minimised to its tray.
// All window operations are performed outside mu: Wails invokes the UI thread
// synchronously, and its click handlers may in turn call back into Desktop.
type Desktop struct {
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
}

//wails:ignore
func NewDesktop(app *application.App, main *application.WebviewWindow) *Desktop {
	d := &Desktop{app: app, main: main}
	d.popup = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "chat-notification", Title: "Typhon", Width: 360, Height: 112,
		Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true,
		BackgroundColour: application.NewRGB(20, 27, 36),
		HTML:             popupHTML("Typhon", ""), AllowSimpleEventEmit: true,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
		Mac:     application.MacWindow{WindowClass: application.MacWindowClassPanel, PanelPreferences: application.MacPanelPreferences{NonActivating: true, FloatingPanel: true, BecomesKeyOnlyIfNeeded: true}, CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary},
	})
	d.unsub = append(d.unsub, app.Event.On("chat:popup-open", func(e *application.CustomEvent) { d.open() }), app.Event.On("chat:popup-close", func(e *application.CustomEvent) { d.Clear() }))
	if f, err := os.CreateTemp("", "typhon-message-*.wav"); err == nil {
		if _, err = f.Write(messageTone()); err == nil {
			d.soundPath = f.Name()
		}
		f.Close()
		if d.soundPath == "" {
			os.Remove(f.Name())
		}
	}
	return d
}

func popupHTML(title, body string) string {
	// No user-controlled HTML, URL or event name is interpolated.
	return `<!doctype html><html><meta charset="utf-8"><style>*{box-sizing:border-box}body{margin:0;background:#141b24;color:#edf2f7;font:14px -apple-system,Segoe UI,sans-serif}button{font:inherit;color:inherit;cursor:pointer}#open{display:block;width:100%;height:112px;text-align:left;background:none;border:1px solid #354558;border-left:3px solid #76b9ff;padding:15px 42px 15px 16px}.brand{font-size:10px;letter-spacing:2px;color:#8c9cac;margin-bottom:7px}b{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-bottom:5px}p{margin:0;color:#becbda;overflow:hidden;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow-wrap:anywhere}#close{position:absolute;right:8px;top:7px;background:none;border:0;font-size:22px;color:#8c9cac}button:focus-visible{outline:2px solid #76b9ff;outline-offset:-3px}</style><button id="open" onclick="window._wails.invoke('wails:event:emit:chat:popup-open')"><div class="brand">TYPHON · CHAT</div><b>` + html.EscapeString(title) + `</b><p>` + html.EscapeString(body) + `</p></button><button id="close" aria-label="Close" onclick="window._wails.invoke('wails:event:emit:chat:popup-close')">×</button></html>`
}

func (d *Desktop) Notify(owner, peer, title, body string) bool {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return false
	}
	d.playLocked()
	d.mu.Unlock()
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
	d.popup.SetHTML(popupHTML(title, body))
	if screen, err := d.main.GetScreen(); err == nil && screen != nil {
		a := screen.WorkArea
		d.popup.SetPosition(a.X+max(0, a.Width-376), a.Y+max(0, a.Height-128))
	}
	showWithoutActivation(d.popup)
	d.mu.Lock()
	if !d.closed && d.serial == serial {
		d.timer = time.AfterFunc(6*time.Second, func() {
			d.mu.Lock()
			current := !d.closed && d.serial == serial
			d.mu.Unlock()
			if current {
				d.Clear()
			}
		})
	}
	d.mu.Unlock()
	return true
}
func (d *Desktop) open() {
	d.mu.Lock()
	owner, peer := d.owner, d.peer
	d.mu.Unlock()
	d.Clear()
	if owner == "" || peer == "" {
		return
	}
	d.main.Restore()
	d.main.Show()
	d.main.Focus()
	d.app.Event.Emit("chat:open", OpenEvent{OwnerID: owner, PeerID: peer})
}
func (d *Desktop) Clear() {
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
		os.Remove(d.soundPath)
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
	ctx, cancel := context.WithCancel(context.Background())
	d.soundCancel = cancel
	d.wg.Add(1)
	go func() { defer d.wg.Done(); defer cancel(); playTone(ctx, d.soundPath) }()
}

// An original, short two-note chime; no external audio asset or codec required.
func messageTone() []byte {
	const rate = 22050
	const count = rate * 36 / 100
	out := make([]byte, 44+count*2)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
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
		for j, f := range []float64{659.25, 987.77} {
			x := t - float64(j)*0.10
			if x >= 0 {
				sample += 0.16 * math.Min(1, x/0.006) * math.Exp(-x*15) * math.Sin(2*math.Pi*f*x)
			}
		}
		binary.LittleEndian.PutUint16(out[44+i*2:], uint16(int16(sample*32767)))
	}
	return out
}
