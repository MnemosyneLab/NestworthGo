package pilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	onboardingReceipt = "onboarding.fixture"
	onboardingKey     = "gio.onboarding.fixture.v1"
	receiptSize       = 32
)

// Pair a database marker with a private receipt and the canonical run root.
// The pair survives onboarding and never depends on a name supplied by the user.
func onboardingMarker(paths Paths, receipt []byte) []byte {
	digest := sha256.New()
	digest.Write([]byte(paths.RootPath()))
	digest.Write([]byte{0})
	digest.Write(receipt)
	return hex.AppendEncode(nil, digest.Sum(nil))
}

func seedOnboardingFixture(ctx context.Context, paths Paths) (result error) {
	receipt := make([]byte, receiptSize)
	if _, err := rand.Read(receipt); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(paths.data, onboardingReceipt), receipt, 0o600); err != nil {
		return err
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	if err := sqlite.NewConfigurationRepository(db).SaveConfiguration(onboardingKey, onboardingMarker(paths, receipt)); err != nil {
		return err
	}
	if err := settings.NewStore(paths.settings).Save(settings.Default()); err != nil {
		return err
	}
	return db.CheckpointWAL(ctx)
}

func verifyIdentity(ctx context.Context, paths Paths, db *sqlite.DB) error {
	path := filepath.Join(paths.data, onboardingReceipt)
	info, err := verifyFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return verifyHousehold(ctx, db)
	}
	invalid := errors.New("the onboarding fixture marker does not match its private run receipt")
	if err != nil || info.Size() != receiptSize {
		return invalid
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.Join(invalid, err, file.Close())
	}
	receipt, err := io.ReadAll(io.LimitReader(file, receiptSize+1))
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	marker, found, err := sqlite.NewConfigurationRepository(db).LoadConfiguration(onboardingKey)
	if err != nil {
		return err
	}
	if len(receipt) != receiptSize || !found || !bytes.Equal(marker, onboardingMarker(paths, receipt)) {
		return invalid
	}
	return nil
}
