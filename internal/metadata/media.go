package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"typhon/internal/storage"
)

const (
	maxImageBytes     = 12 << 20
	maxImageRedirects = 5
)

var (
	errImageURL      = errors.New("некорректный адрес изображения")
	errImageStatus   = errors.New("сервер изображений вернул ошибку")
	errImageTooLarge = errors.New("изображение превышает допустимый размер")
	errImageFormat   = errors.New("формат изображения не поддерживается")
	errAssetPath     = errors.New("некорректный путь ассета")
)

var assetNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type fetchedImage struct {
	Data   []byte
	Format string
	Width  int
	Height int
}

func newImageClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
		},
		CheckRedirect: checkImageRedirect,
	}
}

func checkImageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxImageRedirects {
		return fmt.Errorf("%w: превышено число редиректов (%d)", errImageURL, maxImageRedirects)
	}
	if err := validateImageURLScheme(req.URL); err != nil {
		return err
	}
	prev := via[len(via)-1].URL
	if req.URL.Scheme != prev.Scheme || req.URL.Host != prev.Host {
		req.Header.Del("Authorization")
	}
	return nil
}

func validateImageURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: пустой адрес", errImageURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errImageURL, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: отсутствует хост", errImageURL)
	}
	if err := validateImageURLScheme(u); err != nil {
		return nil, err
	}
	return u, nil
}

func validateImageURLScheme(u *url.URL) error {
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackImageHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("%w: незащищённый http для хоста %q", errImageURL, u.Hostname())
	default:
		return fmt.Errorf("%w: неподдерживаемая схема %q", errImageURL, u.Scheme)
	}
}

func isLoopbackImageHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fetchImage(ctx context.Context, client *http.Client, rawURL string) (fetchedImage, error) {
	u, err := validateImageURL(rawURL)
	if err != nil {
		return fetchedImage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("собрать запрос изображения: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("запросить изображение: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close image response body", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fetchedImage{}, fmt.Errorf("%w: код %d", errImageStatus, resp.StatusCode)
	}

	if resp.ContentLength > maxImageBytes {
		return fetchedImage{}, fmt.Errorf("%w: заявлено %d байт", errImageTooLarge, resp.ContentLength)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return fetchedImage{}, fmt.Errorf("прочитать тело изображения: %w", err)
	}
	if len(data) > maxImageBytes {
		return fetchedImage{}, fmt.Errorf("%w: получено больше %d байт", errImageTooLarge, maxImageBytes)
	}
	if len(data) == 0 {
		return fetchedImage{}, fmt.Errorf("%w: пустое тело ответа", errImageFormat)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fetchedImage{}, fmt.Errorf("%w: %w", errImageFormat, err)
	}
	if format != "jpeg" && format != "png" {
		return fetchedImage{}, fmt.Errorf("%w: %q", errImageFormat, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fetchedImage{}, fmt.Errorf("%w: нулевые размеры изображения", errImageFormat)
	}

	return fetchedImage{
		Data:   data,
		Format: format,
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}

func writeAsset(root, gameID, assetID string, img fetchedImage) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%w: пустой корневой каталог", errAssetPath)
	}
	if !assetNamePattern.MatchString(gameID) {
		return "", fmt.Errorf("%w: некорректный идентификатор игры %q", errAssetPath, gameID)
	}
	if !assetNamePattern.MatchString(assetID) {
		return "", fmt.Errorf("%w: некорректный идентификатор ассета %q", errAssetPath, assetID)
	}
	if len(img.Data) == 0 {
		return "", fmt.Errorf("%w: пустые данные изображения", errAssetPath)
	}

	var ext string
	switch img.Format {
	case "jpeg":
		ext = ".jpg"
	case "png":
		ext = ".png"
	default:
		return "", fmt.Errorf("%w: неизвестный формат изображения %q", errAssetPath, img.Format)
	}

	dir := filepath.Join(root, gamesDirName, gameID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("создать каталог ассетов %s: %w", dir, err)
	}

	fileName := assetID + ext
	target := filepath.Join(dir, fileName)
	if err := storage.WriteAtomic(target, img.Data); err != nil {
		return "", fmt.Errorf("записать ассет %s: %w", target, err)
	}

	return gamesDirName + "/" + gameID + "/" + fileName, nil
}

func isWindowsDrivePath(rel string) bool {
	return len(rel) >= 2 && rel[1] == ':' && isASCIILetter(rel[0])
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func assetPath(root, relPath string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%w: пустой корневой каталог", errAssetPath)
	}
	if relPath == "" {
		return "", fmt.Errorf("%w: пустой путь ассета", errAssetPath)
	}
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") ||
		filepath.VolumeName(relPath) != "" || isWindowsDrivePath(relPath) {
		return "", fmt.Errorf("%w: абсолютный путь ассета %q", errAssetPath, relPath)
	}

	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: путь выходит за пределы каталога %q", errAssetPath, relPath)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errAssetPath, err)
	}
	full := filepath.Join(absRoot, cleanRel)
	if full != absRoot && !strings.HasPrefix(full, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: путь выходит за пределы корня %q", errAssetPath, relPath)
	}
	return full, nil
}
