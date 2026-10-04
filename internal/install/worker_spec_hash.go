package install

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"typhon/internal/uierr"
)

const (
	workerSpecHashFlag = "--spec-sha256"
	maxWorkerSpecBytes = 1 << 20

	// WorkerSpecRejectedExit — код выхода воркера, отказавшегося от задания.
	// Состояние при отказе не пишется: путь к нему лежит в самом задании, а
	// ему как раз не доверяют, и запись по такому пути от имени администратора
	// стала бы перезаписью произвольного файла.
	WorkerSpecRejectedExit = 78
)

var (
	ErrWorkerSpecRejected = uierr.New("install.worker_spec_rejected", "повышенный воркер отказался от задания установки: файл задания не совпал с тем, что записал лаунчер")

	errWorkerSpecHashMissing  = errors.New("лаунчер не передал контрольную сумму задания")
	errWorkerSpecHashInvalid  = errors.New("контрольная сумма задания не SHA-256 в hex")
	errWorkerSpecHashMismatch = errors.New("контрольная сумма файла задания не совпала с переданной")
	errWorkerSpecTooLarge     = errors.New("файл задания больше допустимого размера")
)

func rejectWorkerSpec(cause error) error {
	return fmt.Errorf("%w: %w", ErrWorkerSpecRejected, cause)
}

func workerLaunchArgs(specFile, digest string) []string {
	return []string{installWorkerFlag, specFile, workerSpecHashFlag, digest}
}

// ParseWorkerArgs разбирает аргументы после --install-worker. Отсутствующая
// сумма возвращается пустой строкой: отказ принимает RunWorker, а не разбор.
func ParseWorkerArgs(args []string) (specPath, digest string) {
	if len(args) == 0 {
		return "", ""
	}
	for i := 1; i+1 < len(args); i++ {
		if args[i] == workerSpecHashFlag {
			return args[0], args[i+1]
		}
	}
	return args[0], ""
}

func decodeWorkerSpecDigest(digest string) ([]byte, error) {
	if digest == "" {
		return nil, errWorkerSpecHashMissing
	}
	raw, err := hex.DecodeString(digest)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errWorkerSpecHashInvalid, err)
	}
	if len(raw) != sha256.Size {
		return nil, fmt.Errorf("%w: %d байт вместо %d", errWorkerSpecHashInvalid, len(raw), sha256.Size)
	}
	return raw, nil
}

func readWorkerSpecBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read worker spec %s: %w", path, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxWorkerSpecBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read worker spec %s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close worker spec %s: %w", path, closeErr)
	}
	if len(data) > maxWorkerSpecBytes {
		return nil, fmt.Errorf("worker spec %s: %w", path, errWorkerSpecTooLarge)
	}
	return data, nil
}

// readVerifiedWorkerSpec читает файл один раз и разбирает JSON из тех же
// байт, что прошли проверку: перечитывание после сверки вернуло бы окно для
// подмены. При отказе возвращается нулевая спека, чтобы вызывающий не взял из
// неё ни одного поля.
func readVerifiedWorkerSpec(path, digest string) (workerSpec, error) {
	want, err := decodeWorkerSpecDigest(digest)
	if err != nil {
		return workerSpec{}, rejectWorkerSpec(err)
	}
	data, err := readWorkerSpecBytes(path)
	if errors.Is(err, errWorkerSpecTooLarge) {
		return workerSpec{}, rejectWorkerSpec(err)
	}
	if err != nil {
		return workerSpec{}, err
	}
	got := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return workerSpec{}, rejectWorkerSpec(errWorkerSpecHashMismatch)
	}
	var spec workerSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("parse worker spec %s: %w", path, err)
	}
	return spec, nil
}
