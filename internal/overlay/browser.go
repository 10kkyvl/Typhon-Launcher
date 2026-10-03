package overlay

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"typhon/internal/uierr"
)

const (
	BrowserWindowName = "overlay-browser"

	ErrCodeBrowserURL = "overlay.browser_url"

	browserMaxURL = 2048
	searchURL     = "https://www.google.com/search?q="

	windowNameHeader = "x-wails-window-name"
)

var (
	errNotShown           = errors.New("overlay is not open")
	errBrowserUnsupported = errors.New("browser is not available on this platform")
	errBadBounds          = errors.New("browser area is empty or outside the overlay")
)

type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type browserWindow interface {
	handle() uintptr
	place(r rect) error
	hide()
	navigate(url string)
	back()
	forward()
	reload()
}

func browserURL(input string) (string, error) {
	text := strings.TrimSpace(input)
	if text == "" {
		return "", uierr.New(ErrCodeBrowserURL, "empty address")
	}
	if len(text) > browserMaxURL {
		return "", uierr.New(ErrCodeBrowserURL, fmt.Sprintf("address is longer than %d bytes", browserMaxURL))
	}
	if strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", uierr.New(ErrCodeBrowserURL, "address contains control characters")
	}
	if strings.Contains(text, "://") {
		return checkedURL(text)
	}
	if !strings.ContainsAny(text, " \t") && looksLikeHost(text) {
		return checkedURL("https://" + text)
	}
	return searchURL + url.QueryEscape(text), nil
}

func looksLikeHost(text string) bool {
	host := text
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		return true
	}
	dot := strings.LastIndex(host, ".")
	return dot > 0 && dot < len(host)-1
}

func checkedURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", uierr.Wrap(ErrCodeBrowserURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", uierr.New(ErrCodeBrowserURL, fmt.Sprintf("scheme %q is not allowed", u.Scheme))
	}
	if u.Host == "" {
		return "", uierr.New(ErrCodeBrowserURL, "address has no host")
	}
	if host := strings.ToLower(u.Hostname()); host == "wails.localhost" || strings.HasSuffix(host, ".wails.localhost") {
		return "", uierr.New(ErrCodeBrowserURL, "the launcher's own pages cannot be opened in the browser")
	}
	return u.String(), nil
}

// GuardAssets keeps pages shown in the overlay browser away from the launcher's
// asset server. Wails serves bound methods to any page of a window from the
// same wails.localhost origin and stamps every request with the name of the
// window it came from, which the page itself cannot override.
func GuardAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(windowNameHeader) == BrowserWindowName {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) OpenBrowser(address string, area Bounds) (string, error) {
	target, err := browserURL(address)
	if err != nil {
		return "", err
	}
	err = s.call(func() error {
		win, r, err := s.browserTarget(area)
		if err != nil {
			return err
		}
		win.navigate(target)
		return s.placeBrowser(win, r)
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

func (s *Service) PlaceBrowser(area Bounds) error {
	return s.call(func() error {
		s.mu.Lock()
		open := s.browserOpen
		s.mu.Unlock()
		if !open {
			return nil
		}
		win, r, err := s.browserTarget(area)
		if err != nil {
			return err
		}
		return s.placeBrowser(win, r)
	})
}

func (s *Service) CloseBrowser() error {
	return s.call(func() error {
		s.closeBrowserOnUI()
		return nil
	})
}

func (s *Service) BrowserBack() error {
	return s.withBrowser(browserWindow.back)
}

func (s *Service) BrowserForward() error {
	return s.withBrowser(browserWindow.forward)
}

func (s *Service) BrowserReload() error {
	return s.withBrowser(browserWindow.reload)
}

func (s *Service) withBrowser(action func(browserWindow)) error {
	return s.call(func() error {
		s.mu.Lock()
		win, open := s.browser, s.browserOpen
		s.mu.Unlock()
		if win == nil || !open {
			return errNotShown
		}
		action(win)
		return nil
	})
}

func (s *Service) browserTarget(area Bounds) (browserWindow, rect, error) {
	s.mu.Lock()
	visible, overlay, win, create := s.visible, s.area, s.browser, s.makeBrowser
	s.mu.Unlock()
	if !visible {
		return nil, rect{}, errNotShown
	}
	r, err := browserRect(overlay, area)
	if err != nil {
		return nil, rect{}, err
	}
	if win == nil {
		if create == nil {
			return nil, rect{}, errBrowserUnsupported
		}
		if win, err = create(); err != nil {
			return nil, rect{}, err
		}
		s.mu.Lock()
		s.browser = win
		s.mu.Unlock()
	}
	return win, r, nil
}

func browserRect(overlay rect, area Bounds) (rect, error) {
	if area.Width <= 0 || area.Height <= 0 || area.X < 0 || area.Y < 0 ||
		area.X+area.Width > int(overlay.w) || area.Y+area.Height > int(overlay.h) {
		return rect{}, fmt.Errorf("%w: %+v in %dx%d", errBadBounds, area, overlay.w, overlay.h)
	}
	//nolint:gosec // G115: the area was just checked to lie inside the overlay, whose size is an int32 pixel rectangle.
	return rect{x: overlay.x + int32(area.X), y: overlay.y + int32(area.Y), w: int32(area.Width), h: int32(area.Height)}, nil
}

func (s *Service) placeBrowser(win browserWindow, r rect) error {
	if err := win.place(r); err != nil {
		win.hide()
		s.mu.Lock()
		s.browserOpen = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.browserOpen = true
	s.mu.Unlock()
	return nil
}

func (s *Service) closeBrowserOnUI() {
	s.mu.Lock()
	win := s.browser
	s.browserOpen = false
	s.mu.Unlock()
	if win != nil {
		win.hide()
	}
}

// The page stays loaded while the overlay is closed, so a video or a stream
// keeps playing; the browser shows again only when the panel asks for it.
func (s *Service) hideBrowserOnUI() {
	s.mu.Lock()
	win := s.browser
	s.mu.Unlock()
	if win != nil {
		win.hide()
	}
}

func (s *Service) ownWindow(hwnd uintptr) bool {
	s.mu.Lock()
	win, browser := s.win, s.browser
	s.mu.Unlock()
	if win != nil && hwnd == win.handle() {
		return true
	}
	return browser != nil && hwnd == browser.handle()
}
