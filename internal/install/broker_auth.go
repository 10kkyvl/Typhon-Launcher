package install

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// The private key exists only in the launcher. The elevated process receives
// the public key in its launch arguments, never from the writable queue.
func writeSignedBrokerSpec(ctx context.Context, dir string, spec workerSpec, key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errBrokerOutsidePin
	}
	f, err := os.Open(spec.InstallerPath)
	if err != nil {
		return err
	}
	digest, readErr := hashBrokerInstaller(ctx, f)
	closeErr := f.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
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
	// Resolve before opening and launch that same path. On Windows the held
	// handle denies writes/deletion until the installer has finished.
	path, err := filepath.EvalSymlinks(spec.InstallerPath)
	if err != nil {
		return err
	}
	f, err := openBrokerInstaller(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close broker installer", "error", err)
		}
	}()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	if hex.EncodeToString(sum.Sum(nil)) != spec.InstallerSHA256 {
		return fmt.Errorf("%w: installer content changed", errBrokerOutsidePin)
	}
	spec.InstallerPath = path
	return runWorkerSpec(spec)
}
