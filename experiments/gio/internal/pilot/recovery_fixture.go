package pilot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	recoveryReceipt   = "recovery.fixture"
	recoveryMarkerKey = "gio.recovery.fixture.v1"
	recoveryBackup    = "recovery-sample" + backup.FileExt
)

var recoverySafetyName = regexp.MustCompile(`^nestworth\.db\.pre-restore-[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}$`)

type recoverySeal struct {
	Token    []byte `json:"token"`
	Journal  string `json:"journal_sha256"`
	Safety   string `json:"safety_sha256"`
	Settings string `json:"settings_sha256"`
}

// PrepareRecovery creates a fresh synthetic profile stopped after the shared
// restore installer has swapped in its replacement, just before startup finalization.
func PrepareRecovery(ctx context.Context, paths Paths) error {
	if err := requireDisposableRun(paths); err != nil {
		return err
	}
	return publishFixture(ctx, paths, false, prepareRecoveryCandidate)
}

func prepareRecoveryCandidate(ctx context.Context, paths Paths) (result error) {
	if err := prepareCandidate(ctx, paths, seedFixture); err != nil {
		return err
	}
	_, unlock, err := lockVerifiedFixture(ctx, paths, false)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, unlock()) }()

	token := make([]byte, receiptSize)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	pkg, err := makeRecoveryBackup(ctx, paths, token)
	if err != nil {
		return err
	}
	if err := backup.InstallRestore(ctx, paths.database, pkg, nil, backup.RestoreClasses{Chrome: true, Format: true}, historyStart().AddDate(0, 0, 21)); err != nil {
		return err
	}
	journal, err := backup.ReadJournal(paths.database)
	if err != nil {
		return err
	}
	raw, err := readRecoveryFile(backup.JournalPath(paths.database))
	if err != nil {
		return err
	}
	safety, err := openRecoveryFile(filepath.Join(paths.data, journal.SafetyName))
	if err != nil {
		return err
	}
	digest := sha256.New()
	_, err = io.Copy(digest, safety)
	if err := errors.Join(err, safety.Close()); err != nil {
		return err
	}
	prefs, err := readRecoveryFile(paths.settings)
	if err != nil {
		return err
	}
	seal, err := jsoncodec.Marshal(recoverySeal{
		Token: token, Journal: recoveryHash(raw), Safety: hex.EncodeToString(digest.Sum(nil)), Settings: recoveryHash(prefs),
	})
	if err != nil {
		return err
	}
	if err := backup.WriteAtomicFile(filepath.Join(paths.data, recoveryReceipt), seal, 0o600); err != nil {
		return err
	}
	_, err = checkedRecoveryJournal(ctx, paths)
	return err
}

func makeRecoveryBackup(ctx context.Context, paths Paths, token []byte) (result backup.Package, err error) {
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return backup.Package{}, err
	}
	defer func() { err = errors.Join(err, db.CheckpointWAL(ctx), db.Close()) }()
	repo := sqlite.NewRepository(db)
	svc := application.NewService(repo)
	svc.SetClock(func() time.Time { return historyStart().AddDate(0, 0, 20) })
	svc.SetBackup(backup.NewRuntime())
	svc.SetLiveDatabasePath(paths.database)
	if err := sqlite.NewConfigurationRepository(db).SaveConfiguration(recoveryMarkerKey, onboardingMarker(paths, token)); err != nil {
		return backup.Package{}, err
	}
	core, err := svc.Bootstrap(ctx)
	if err != nil {
		return backup.Package{}, err
	}
	row, err := svc.CreateAccount(ctx, application.AccountInput{
		Name: "Recovery Sample Reserve", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "42.25",
		IncludeInNetWorth: true, IncludeInLiquidAssets: true, OwnerIDs: []domain.MemberID{core.Members[0].ID},
	})
	if err != nil {
		return backup.Package{}, err
	}
	prefs := settings.Default()
	prefs.Language, prefs.Appearance, prefs.Accent, prefs.DateFormat = settings.LanguageEnglish, settings.AppearanceDark, settings.AccentMint, settings.DateFormatDayFirst
	presentation, err := settings.PresentationJSON(prefs)
	if err != nil {
		return backup.Package{}, err
	}
	filename := filepath.Join(paths.data, recoveryBackup)
	if _, err := svc.CreateBackup(ctx, filename, presentation); err != nil {
		return backup.Package{}, err
	}
	prefs.Appearance, prefs.Accent, prefs.DateFormat = settings.AppearanceLight, settings.AccentIndigo, settings.DateFormatISO
	if err := settings.NewStore(paths.settings).Save(prefs); err != nil {
		return backup.Package{}, err
	}
	svc.SetClock(func() time.Time { return historyStart().AddDate(0, 0, 21) })
	if _, err := svc.AppendAccountValue(ctx, row.Account.ID, "99.50", ""); err != nil {
		return backup.Package{}, err
	}
	return backup.ReadPackage(filename)
}

func reconcileRecoveryFixture(ctx context.Context, paths Paths) error {
	if err := requireDisposableRun(paths); err != nil {
		return errors.Join(ErrRecoveryPending, err)
	}
	journal, err := checkedRecoveryJournal(ctx, paths)
	if err != nil {
		return errors.Join(ErrRecoveryPending, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := backup.ReconcileJournalOnStartup(paths.database, settings.NewStore(paths.settings), journal); err != nil {
		return errors.Join(ErrRecoveryPending, err)
	}
	return nil
}

// The starting disk layout is intentionally narrow. An installed replacement
// and its safety copy must each be self-contained. The journal cannot select
// any additional source or destination, and the ordinary profile never opts in.
func checkedRecoveryJournal(ctx context.Context, paths Paths) (backup.Journal, error) {
	var journal backup.Journal
	data, err := readRecoveryFile(filepath.Join(paths.data, recoveryReceipt))
	if err != nil {
		return journal, fmt.Errorf("read disposable recovery receipt: %w", err)
	}
	var seal recoverySeal
	if err := jsoncodec.Unmarshal(data, &seal); err != nil || len(seal.Token) != receiptSize {
		return journal, errors.New("invalid disposable recovery receipt")
	}
	raw, err := readRecoveryFile(backup.JournalPath(paths.database))
	if err != nil {
		return journal, fmt.Errorf("read disposable restore journal: %w", err)
	}
	if err := jsoncodec.Unmarshal(raw, &journal); err != nil {
		return backup.Journal{}, errors.New("invalid disposable restore journal")
	}
	if journal.FormatVersion != backup.FormatVersion || journal.BackupFormat != backup.FormatVersion ||
		journal.SchemaVersion != sqlite.CurrentSchemaVersion || journal.Operation != "restore" ||
		journal.State != backup.StateReplacementInstalled || journal.LiveName != filepath.Base(paths.database) ||
		journal.StagingName != backup.StagingName(paths.database) || !recoverySafetyName.MatchString(journal.SafetyName) ||
		journal.FailedName != "" || journal.OriginalPresent == nil || !*journal.OriginalPresent ||
		!journal.RestoreChrome || !journal.RestoreFormat || journal.RestoreRouting {
		return backup.Journal{}, errors.New("restore journal is outside the supported disposable recovery layout")
	}
	if recoveryHash(raw) != seal.Journal {
		return backup.Journal{}, errors.New("restore journal does not match the disposable recovery receipt")
	}
	prefs, err := readRecoveryFile(paths.settings)
	if err != nil || recoveryHash(prefs) != seal.Settings {
		return backup.Journal{}, errors.New("initial settings do not match the disposable recovery receipt")
	}
	if _, status, err := settings.NewStore(paths.settings).LoadWithStatus(); err != nil || status != settings.LoadStatusClean {
		return backup.Journal{}, errors.New("initial disposable recovery settings are invalid")
	}
	safety := filepath.Join(paths.data, journal.SafetyName)
	staging := filepath.Join(paths.data, journal.StagingName)
	if err := requireRecoveryPathsAbsent(paths, safety, staging); err != nil {
		return backup.Journal{}, err
	}
	for _, candidate := range []struct{ path, digest string }{
		{paths.database, journal.BackupSHA256}, {safety, seal.Safety},
	} {
		if err := verifyRecoveryDatabase(ctx, paths, candidate.path, candidate.digest, seal.Token); err != nil {
			return backup.Journal{}, err
		}
	}
	return journal, nil
}

func requireRecoveryPathsAbsent(paths Paths, safety, staging string) error {
	absent := []string{staging, filepath.Join(paths.data, onboardingReceipt)}
	for _, main := range []string{paths.database, safety, staging} {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			absent = append(absent, main+suffix)
		}
	}
	for _, path := range absent {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("unexpected file in disposable recovery layout: %s", filepath.Base(path))
		}
	}
	return nil
}

func verifyRecoveryDatabase(ctx context.Context, paths Paths, source, digest string, token []byte) error {
	file, err := openRecoveryFile(source)
	if err != nil {
		return err
	}
	defer file.Close()
	dir, err := os.MkdirTemp("", "nestworth-gio-recovery-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "verify.db")
	snapshot, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(snapshot, hash), io.LimitReader(file, backup.MaxMemberBytes+1))
	if err := errors.Join(err, snapshot.Close()); err != nil {
		return err
	}
	if n > backup.MaxMemberBytes || hex.EncodeToString(hash.Sum(nil)) != digest {
		return fmt.Errorf("database does not match disposable recovery receipt: %s", filepath.Base(source))
	}
	db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := verifyHousehold(ctx, db); err != nil {
		return err
	}
	marker, found, err := sqlite.NewConfigurationRepository(db).LoadConfiguration(recoveryMarkerKey)
	if err != nil || !found || string(marker) != string(onboardingMarker(paths, token)) {
		return errors.New("database is not paired with this disposable recovery run")
	}
	return nil
}

func recoveryHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readRecoveryFile(path string) ([]byte, error) {
	const limit = 64 * 1024
	file, err := openRecoveryFile(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err := errors.Join(err, file.Close()); err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errors.New("disposable recovery metadata is too large")
	}
	return data, nil
}

func openRecoveryFile(path string) (*os.File, error) {
	info, err := verifyFile(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("a disposable recovery file changed during verification"), err, file.Close())
	}
	return file, nil
}
