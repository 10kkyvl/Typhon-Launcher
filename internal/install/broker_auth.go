package install

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/uierr"
)

var (
	errInstallerChanged     = uierr.New("install.installer_changed", "установщик изменился после запроса прав администратора, запуск отменён. Повторите установку")
	errInstallerHashMissing = errors.New("в задании нет контрольной суммы установщика")
)

// The private key exists only in the launcher. The elevated process receives
// the public key in its launch arguments, never from the writable queue.
func writeSignedBrokerSpec(ctx context.Context, dir string, spec workerSpec, key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errBrokerOutsidePin
	}
	digest, err := installerDigest(ctx, spec.InstallerPath)
	if err != nil {
		return err
	}
	spec.InstallerSHA256 = digest
	spec.BrokerSignature = ""
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	spec.BrokerSignature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, data))
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeWorkerSpec(brokerSpecPath(dir), spec)
}

func installerDigest(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errEmptyInstallerPath
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	digest, readErr := hashBrokerInstaller(ctx, f)
	closeErr := f.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return digest, nil
}

// Bound each read so cancellation does not wait for hashing the entire installer.
func hashBrokerInstaller(ctx context.Context, src io.Reader) (string, error) {
	sum := sha256.New()
	if err := copyStream(ctx, sum, src, &reporter{}, make([]byte, 128*1024)); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func verifyBrokerSignature(spec workerSpec, keys []string) error {
	if len(keys) != 1 {
		return errBrokerOutsidePin
	}
	key, err := base64.StdEncoding.DecodeString(keys[0])
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errBrokerOutsidePin
	}
	signature, err := base64.StdEncoding.DecodeString(spec.BrokerSignature)
	if err != nil {
		return errBrokerOutsidePin
	}
	spec.BrokerSignature = ""
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, data, signature) {
		return errBrokerOutsidePin
	}
	return nil
}

func runSignedBrokerSpec(spec workerSpec) error {
	err := runWorkerSpec(spec)
	if errors.Is(err, errInstallerChanged) {
		return fmt.Errorf("%w: %w", errBrokerOutsidePin, err)
	}
	return err
}

// The installer sits in a folder the unelevated user can write to. Resolve
// before opening and launch that same path: on Windows the held handle denies
// writes and deletion until the installer has finished.
func pinInstaller(ctx context.Context, spec workerSpec) (*os.File, workerSpec, error) {
	if spec.InstallerSHA256 == "" {
		return nil, workerSpec{}, errInstallerHashMissing
	}
	path, err := filepath.EvalSymlinks(spec.InstallerPath)
	if err != nil {
		return nil, workerSpec{}, err
	}
	f, err := openBrokerInstaller(path)
	if err != nil {
		return nil, workerSpec{}, err
	}
	digest, err := hashBrokerInstaller(ctx, f)
	if err != nil {
		closePinnedInstaller(f)
		return nil, workerSpec{}, err
	}
	if digest != spec.InstallerSHA256 {
		closePinnedInstaller(f)
		return nil, workerSpec{}, errInstallerChanged
	}
	spec.InstallerPath = path
	return f, spec, nil
}

func closePinnedInstaller(f *os.File) {
	if err := f.Close(); err != nil {
		slog.Warn("close pinned installer", "error", err)
	}
}
