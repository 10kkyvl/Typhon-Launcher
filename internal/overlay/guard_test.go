package overlay

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeNativeWindow struct {
	application.Window
	fullscreen bool
	scripts    []string
}

func (w *fakeNativeWindow) IsFullscreen() bool { return w.fullscreen }

func (w *fakeNativeWindow) ExecJS(js string) { w.scripts = append(w.scripts, js) }

func TestBrowserWindowRunsTheGuardInEveryDocument(t *testing.T) {
	opts := browserOptions(func() {})
	if opts.HTML == "" {
		t.Fatal("Wails registers WebviewWindowOptions.JS as a document-created script only for a window created with HTML")
	}
	if opts.JS != browserGuard {
		t.Fatalf("window script = %q, want the browser guard", opts.JS)
	}
	if opts.URL != "" {
		t.Fatalf("URL = %q: the first document must come from HTML so the guard is registered before it", opts.URL)
	}
	if opts.AllowSimpleEventEmit {
		t.Fatal("pages in the browser must not be able to emit launcher events")
	}
	if opts.Name != BrowserWindowName {
		t.Fatalf("name = %q, GuardAssets keys on %q", opts.Name, BrowserWindowName)
	}
	for _, kind := range []application.PermissionType{
		application.PermissionMicrophone, application.PermissionCamera, application.PermissionGeolocation,
		application.PermissionNotifications, application.PermissionClipboardRead,
	} {
		if opts.Permissions[kind] != application.PermissionDeny {
			t.Errorf("permission %d = %v, want denied", kind, opts.Permissions[kind])
		}
	}
}

func TestBrowserGuard(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"launcher hook is pinned to an empty object without a prototype", "Object.defineProperty(window,'_wails',{value:Object.freeze(Object.create(null)),writable:false,configurable:false,enumerable:false});"},
		{"window.open cannot be put back", "Object.defineProperty(window,'open',{get:function(){return open;},set:function(){},enumerable:true,configurable:false});"},
		{"links are caught before the page sees them", "addEventListener('click',onLink,true);"},
		{"middle clicks are caught too", "addEventListener('auxclick',onLink,true);"},
		{"forms are caught before they submit", "addEventListener('submit',onSubmit,true);"},
		{"script submits go through the same check", "HTMLFormElement.prototype.submit=function(){"},
		{"only web addresses are followed", "u.protocol==='http:'||u.protocol==='https:'"},
		{"links are known by tag name, so svg and other realms count", "if(t==='a'||t==='area')return path[i];"},
		{"forms are known by tag name, so other realms count", "if(tag(form)==='form'&&!aim(form,event.submitter))"},
		{"svg links may carry the address in xlink:href", "getAttrNS.call(link,'http://www.w3.org/1999/xlink','href')"},
		{"tag names come from the prototype getter, not the element", "localName=Object.getOwnPropertyDescriptor(Element.prototype,'localName').get"},
		{"every shadow root is created open", "opts.mode='open';"},
		{"attachShadow cannot be put back", "Object.defineProperty(Element.prototype,'attachShadow',{get:function(){return attach;},set:function(){},enumerable:true,configurable:false});"},
		{"forms inside a shadow root are caught there", "listen.call(root,'submit',onSubmit,true);"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(browserGuard, c.want) {
				t.Fatalf("guard does not contain %q", c.want)
			}
		})
	}
	if !strings.HasPrefix(browserGuard, cases[0].want) {
		t.Fatal("the launcher hook must be pinned before anything else in the guard can fail")
	}
	if strings.Contains(browserGuard, "instanceof") {
		t.Fatal("instanceof fails for elements made in another frame, which then open a new window")
	}
}

func TestEscapeInTheBrowser(t *testing.T) {
	cases := []struct {
		name       string
		fullscreen bool
		wantHides  int
		wantJS     bool
	}{
		{"page in its panel closes the overlay", false, 1, false},
		{"fullscreen video leaves fullscreen first", true, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hides := 0
			opts := browserOptions(func() { hides++ })
			bind := opts.KeyBindings["escape"]
			if bind == nil {
				t.Fatalf("no Esc binding in %v", opts.KeyBindings)
			}
			win := &fakeNativeWindow{fullscreen: c.fullscreen}
			bind(win)
			if hides != c.wantHides {
				t.Fatalf("overlay hidden %d times, want %d", hides, c.wantHides)
			}
			if got := len(win.scripts) == 1 && strings.Contains(win.scripts[0], "exitFullscreen()"); got != c.wantJS {
				t.Fatalf("scripts = %q, want exitFullscreen: %v", win.scripts, c.wantJS)
			}
		})
	}
}
