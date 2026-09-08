//go:build darwin && !devmock

package selfupdate

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"typhon/internal/settings"
)

// bundleCycleServer поднимает httptest-сервер, отдающий подписанный манифест
// и сам архив обновления по тем же путям, что и настоящий бэкенд: /launcher/
// manifest и URL артефакта. manifestBody и artifactBody читаются на каждый
// запрос — тест выставляет их через возвращённые setter'ы до первого вызова
// клиента, поэтому гонки с обработчиком нет.
// newClientWithKey строит Client, который проверяет подпись манифеста
// заданным ключом вместо прод-ключа. Не экспортируется и не участвует в
// NewClient намеренно: это единственная точка, где ключ проверки подписи
// можно заменить, и она закрыта для всего, что лежит за пределами пакета
// selfupdate. Так тест может прогнать весь цикл (манифест → подпись →
// скачивание → применение) на одноразовой паре ключей, а прод-путь
// (NewClient, PublicKey()) остаётся тем же самым кодом без единой лазейки.
func newClientWithKey(baseURL string, key ed25519.PublicKey) (*Client, error) {
	c, err := NewClient(baseURL)
	if err != nil {
		return nil, err
	}
	c.key = key
	return c, nil
}

func bundleCycleServer(t *testing.T) (srv *httptest.Server, setManifest, setArtifact func([]byte), artifactName string) {
	t.Helper()
	const name = "typhon-darwin-bundle.zip"
	var manifestBody, artifactBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc(ManifestPath, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(manifestBody); err != nil {
			t.Errorf("write manifest response: %v", err)
		}
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(artifactBody); err != nil {
			t.Errorf("write artifact response: %v", err)
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func(b []byte) { manifestBody = b }, func(b []byte) { artifactBody = b }, name
}

// bundleCycleFixture готовит бандл-архив нового релиза, отдаёт его серверу
// через setArtifact и строит артефакт для подписи манифеста: настоящий zip,
// собранный тем же helper'ом, что и bundle_darwin_test.go, и настоящий
// sha256/размер этого архива — как их вычислял бы реальный релизный скрипт.
func bundleCycleFixture(t *testing.T, srv *httptest.Server, setArtifact func([]byte), name, exeBody string) ([]byte, Artifact) {
	t.Helper()
	zipPath := appZip(t, exeBody)
	body, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", zipPath, err)
	}
	setArtifact(body)
	art := Artifact{
		OS:     "darwin",
		Arch:   runtime.GOARCH,
		Kind:   KindBundle,
		Name:   name,
		URL:    srv.URL + "/" + name,
		Size:   int64(len(body)),
		SHA256: sha256Hex(t, body),
	}
	return body, art
}

// bundleCycleService строит Service так же напрямую, как service_apply_test.go,
// но с client'ом, который проверяет подпись manifest'а checkKey вместо
// вшитого прод-ключа — единственный способ дать всему циклу собственную пару
// ключей без единой изменённой строки в apply_darwin.go.
func bundleCycleService(t *testing.T, srv *httptest.Server, checkKey ed25519.PublicKey, configDir, currentVersion string) *Service {
	t.Helper()
	client, err := newClientWithKey(srv.URL, checkKey)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	return &Service{
		dir:            configDir,
		client:         client,
		store:          mustStore(t, configDir),
		notes:          mustNotesStore(t, configDir),
		currentVersion: currentVersion,
	}
}

// TestBundleUpdateFullCycle прогоняет весь путь самообновления под macOS на
// настоящих функциях, без единого мока применения: проверка манифеста по
// сети → проверка подписи → скачивание артефакта → Apply из
// apply_darwin.go → подмена бандла на диске. До этого теста этот путь для
// KindBundle не гонялся вовсе: Client.PublicKey() в проде вшит и подменить
// его для теста было нечем, потому что apply_darwin.go не собирается под
// devmock, а подменяемый ключ жил только под devmock.
func TestBundleUpdateFullCycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir, err := settings.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}

	priv, pub := testKeyPair(t)
	srv, setManifest, setArtifact, artifactName := bundleCycleServer(t)
	body, art := bundleCycleFixture(t, srv, setArtifact, artifactName, "new bundle contents")

	const newVersion = "9.9.9"
	m := Manifest{Version: newVersion, PublishedAt: time.Now(), Artifacts: []Artifact{art}}
	signed, err := SignManifest(m, priv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	setManifest(signed)

	s := bundleCycleService(t, srv, pub, configDir, "1.0.0")

	status, err := s.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if status.State != StateAvailable || status.AvailableVersion != newVersion {
		t.Fatalf("status after check = %+v, want StateAvailable/%s", status, newVersion)
	}

	status, err = s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}
	if status.State != StateReady {
		t.Fatalf("status after download = %+v, want StateReady", status)
	}
	archivePath := s.readyPath
	if archivePath == "" {
		t.Fatal("readyPath is empty after a successful download")
	}
	downloaded, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", archivePath, err)
	}
	if string(downloaded) != string(body) {
		t.Fatal("downloaded artifact differs from what the server served")
	}

	apps := filepath.Join(home, "Applications")
	exe := filepath.Join(apps, "Typhon.app", "Contents", "MacOS", "typhon")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Применение идёт через тот же Apply, что и на настоящем устройстве
	// пользователя: не через Service.ApplyUpdate (который поднял бы
	// отдельный процесс воркера), а прямым вызовом package-функции.
	if err := Apply(context.Background(), archivePath, filepath.Dir(exe), exe); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile(%s) after apply: %v", exe, err)
	}
	if string(got) != "new bundle contents" {
		t.Fatalf("after apply = %q, want %q: старый бандл не заменился", string(got), "new bundle contents")
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("режим = %v, исполняемый бит потерян после применения", info.Mode())
	}
	entries, err := os.ReadDir(apps)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "Typhon.app" {
		t.Fatalf("рядом с бандлом остался мусор: %v", entries)
	}
}

// TestBundleUpdateRejectsForeignSignature — обратная сторона предыдущего
// теста и главная проверка шва: манифест подписан НЕ тем ключом, которым
// клиент проверяет подпись. Если шов в client.go когда-нибудь потеряет
// проверку и начнёт доверять c.key безусловно или использовать не тот ключ,
// этот тест обязан упасть первым — раньше, чем это увидит пользователь.
func TestBundleUpdateRejectsForeignSignature(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir, err := settings.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}

	foreignPriv, _ := testKeyPair(t)
	_, legitPub := testKeyPair(t)
	srv, setManifest, setArtifact, artifactName := bundleCycleServer(t)
	_, art := bundleCycleFixture(t, srv, setArtifact, artifactName, "attacker bundle contents")

	const newVersion = "9.9.9"
	m := Manifest{Version: newVersion, PublishedAt: time.Now(), Artifacts: []Artifact{art}}
	signed, err := SignManifest(m, foreignPriv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	setManifest(signed)

	s := bundleCycleService(t, srv, legitPub, configDir, "1.0.0")

	apps := filepath.Join(home, "Applications")
	exe := filepath.Join(apps, "Typhon.app", "Contents", "MacOS", "typhon")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := s.CheckForUpdate(context.Background()); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("CheckForUpdate() error = %v, want ErrBadSignature", err)
	}
	if got := s.GetStatus(); got.State == StateReady || got.State == StateAvailable {
		t.Fatalf("status после отказа проверки подписи = %+v, обновление не должно считаться найденным", got)
	}

	if _, err := s.DownloadUpdate(context.Background()); !errors.Is(err, errNoUpdateChecked) {
		t.Fatalf("DownloadUpdate() error = %v, want errNoUpdateChecked: скачивание не должно начинаться без проверенного манифеста", err)
	}

	// Даже если бы файл артефакта каким-то образом оказался на диске в
	// каталоге кеша (например, из прежней попытки), Apply обязан отказать:
	// store не отмечал ничего готовым, потому что до сохранения состояния
	// цикл не дошёл — проверка подписи отвергла манифест раньше.
	dummyArchive := filepath.Join(configDir, "selfupdate", newVersion, artifactName)
	if err := os.MkdirAll(filepath.Dir(dummyArchive), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(dummyArchive, []byte("should never be applied"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Apply(context.Background(), dummyArchive, filepath.Dir(exe), exe); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Apply() error = %v, want ErrNotReady: применять нечего, скачивания не было", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", exe, err)
	}
	if string(got) != "old binary" {
		t.Fatal("бандл подменился, хотя манифест был подписан чужим ключом")
	}
}
