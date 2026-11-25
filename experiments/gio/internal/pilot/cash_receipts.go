package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const cashReceiptDirectory = "cash-receipts"

type PendingCashReceipt struct {
	MutationID, SHA256 string
	MatchesLedger      bool
}

type cashReceiptFile struct {
	MutationID string `json:"mutationId"`
	SHA256     string `json:"sha256"`
	Ledger     string `json:"ledger"`
}

// CashReceiptStore is available only while its verified pilot session owns the
// instance lock. Each private file represents one user-confirmed intention.
type CashReceiptStore struct {
	mu       sync.Mutex
	closed   bool
	paths    Paths
	verified os.FileInfo
	db       *sqlite.DB
}

func (s *CashReceiptStore) directory() (string, error) {
	if s.closed {
		return "", errors.New("cash receipts require an open pilot session")
	}
	info, err := os.Stat(s.paths.database)
	if err != nil || s.verified == nil || !os.SameFile(s.verified, info) {
		return "", errors.New("the open ledger changed")
	}
	return filepath.Join(s.paths.RootPath(), cashReceiptDirectory), nil
}

func (s *CashReceiptStore) binding(ctx context.Context) (string, error) {
	household, err := sqlite.NewRepository(s.db).Household(ctx)
	if err != nil {
		return "", err
	}
	if household == nil {
		return "", errors.New("cash receipts require a household")
	}
	stat := s.verified.Sys().(*syscall.Stat_t)
	// Backup installation replaces the database file, even for the same
	// household. An in-place rollback still needs a matching database receipt.
	data := fmt.Sprintf("%s\x00%s\x00%d\x00%d", s.paths.RootPath(), household.ID, stat.Dev, stat.Ino)
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:]), nil
}

func validReceiptID(id string) bool {
	parsed, err := domain.ParseMutationID(id)
	return err == nil && parsed.String() == id
}

func validReceiptDigest(digest string) bool {
	_, err := hex.DecodeString(digest)
	return len(digest) == sha256.Size*2 && err == nil && digest == strings.ToLower(digest)
}

// Remember must succeed before dispatch. Repeating the same frozen intention
// preserves its original identity and also retries the directory flush.
func (s *CashReceiptStore) Remember(ctx context.Context, mutationID, digest string) error {
	if !validReceiptID(mutationID) || !validReceiptDigest(digest) {
		return errors.New("invalid cash receipt identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.directory()
	if err != nil {
		return err
	}
	binding, err := s.binding(ctx)
	if err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := requirePrivateReceiptDirectory(dir); err != nil {
		return err
	}
	if err := syncReceiptDirectory(s.paths.RootPath()); err != nil {
		return err
	}
	path := filepath.Join(dir, mutationID+".json")
	record := cashReceiptFile{MutationID: mutationID, SHA256: digest, Ledger: binding}
	if old, err := readCashReceipt(path); !errors.Is(err, os.ErrNotExist) {
		if err != nil || old != record {
			return errors.New("cash receipt identity already exists with different or unreadable data")
		}
		return syncReceiptDirectory(dir)
	}
	raw, err := jsoncodec.Marshal(record)
	if err != nil {
		return err
	}
	if err := backup.WriteAtomicFile(path, raw, 0o600); err != nil {
		return err
	}
	// Dispatch requires a confirmed directory flush, including on retry.
	return syncReceiptDirectory(dir)
}

func readCashReceipt(path string) (cashReceiptFile, error) {
	var record cashReceiptFile
	info, err := verifyReceiptFile(path)
	if err != nil {
		return record, err
	}
	file, err := os.Open(path)
	if err != nil {
		return record, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return record, errors.New("cash receipt file changed while opening")
	}
	const maxSize = 512
	raw, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil || len(raw) > maxSize {
		return record, errors.New("could not read cash receipt identity")
	}
	if err := jsoncodec.Unmarshal(raw, &record); err != nil {
		return record, err
	}
	return record, nil
}

func (s *CashReceiptStore) Pending(ctx context.Context) ([]PendingCashReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.directory()
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err := requirePrivateReceiptDirectory(dir); err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var pending []PendingCashReceipt
	for _, entry := range files {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if ok && validReceiptID(id) {
			pending = append(pending, PendingCashReceipt{MutationID: id})
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}
	binding, err := s.binding(ctx)
	if err != nil {
		return nil, err
	}
	for i, entry := range pending {
		file, err := readCashReceipt(filepath.Join(dir, entry.MutationID+".json"))
		if err == nil && file.MutationID == entry.MutationID && validReceiptDigest(file.SHA256) && file.Ledger == binding {
			pending[i].SHA256, pending[i].MatchesLedger = file.SHA256, true
		}
	}
	return pending, nil
}

// Forget acknowledges a notice in the UI. It only removes the private record.
func (s *CashReceiptStore) Forget(id string) error {
	if !validReceiptID(id) {
		return errors.New("invalid cash receipt identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.directory()
	if err != nil {
		return err
	}
	if err := requirePrivateReceiptDirectory(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	if _, err := verifyReceiptFile(path); err == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncReceiptDirectory(dir)
}

func syncReceiptDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func requirePrivateReceiptDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || !isPrivateOwned(info) {
		return fmt.Errorf("the cash receipt store requires a private directory owned by the current user: %s", path)
	}
	return nil
}

func isPrivateOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0o077 == 0
}

func verifyReceiptFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !isPrivateOwned(info) || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, errors.New("expected a private regular file without symbolic or hard links")
	}
	return info, nil
}
