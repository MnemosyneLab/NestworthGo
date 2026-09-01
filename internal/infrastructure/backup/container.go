// Package backup owns the on-disk backup container, restore journal, and
// last-backup status sidecar. It does not open the live business database.
package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/version"
)

const (
	FormatVersion   = 1
	BackupKindFull  = "full"
	MemberManifest  = "manifest.json"
	MemberDatabase  = "database.sqlite"
	MemberSettings  = "settings.json"
	MaxMemberBytes  = 1 << 30
	MaxTotalBytes   = (1 << 30) + (1 << 20)
	MaxManifestSize = 64 << 10
	MaxSettingsSize = 1 << 20
	FileExt         = ".nestworth-backup"
)

type MemberMeta struct {
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type Manifest struct {
	FormatVersion int                   `json:"format_version"`
	BackupKind    string                `json:"backup_kind"`
	AppName       string                `json:"app_name"`
	AppVersion    string                `json:"app_version"`
	AppBuild      string                `json:"app_build"`
	SchemaVersion int                   `json:"schema_version"`
	CreatedAt     string                `json:"created_at"`
	Members       map[string]MemberMeta `json:"members"`
	RowCounts     sqlite.EntityCounts   `json:"row_counts"`
}

type Package struct {
	Manifest Manifest
	Database []byte
	Settings []byte
}

func NewManifest(createdAt time.Time, database, settings []byte, counts sqlite.EntityCounts) Manifest {
	return Manifest{
		FormatVersion: FormatVersion,
		BackupKind:    BackupKindFull,
		AppName:       version.Name,
		AppVersion:    strings.TrimPrefix(version.Version, "v"),
		AppBuild:      version.Build,
		SchemaVersion: sqlite.CurrentSchemaVersion,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		Members: map[string]MemberMeta{
			MemberDatabase: {SHA256: sha256Hex(database), SizeBytes: int64(len(database))},
			MemberSettings: {SHA256: sha256Hex(settings), SizeBytes: int64(len(settings))},
		},
		RowCounts: counts,
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func WritePackage(dest string, pkg Package) error {
	if !strings.HasSuffix(strings.ToLower(dest), FileExt) {
		return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup file must use the .nestworth-backup extension"}
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup directory could not be created"}
	}
	manifestBytes, err := json.MarshalIndent(pkg.Manifest, "", "  ")
	if err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup manifest could not be encoded"}
	}
	manifestBytes = append(manifestBytes, '\n')
	if int64(len(manifestBytes)) > MaxManifestSize {
		return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup manifest exceeds the size limit"}
	}
	tmp, err := os.CreateTemp(dir, ".nestworth-backup-*.tmp")
	if err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup temporary file could not be created"}
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup permissions could not be set"}
	}
	zipWriter := zip.NewWriter(tmp)
	for _, member := range []struct {
		name string
		data []byte
	}{
		{MemberManifest, manifestBytes},
		{MemberDatabase, pkg.Database},
		{MemberSettings, pkg.Settings},
	} {
		header := &zip.FileHeader{Name: member.name, Method: zip.Deflate}
		header.SetMode(0o600)
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = zipWriter.Close()
			_ = tmp.Close()
			return &domain.Error{Code: domain.ErrUnavailable, Message: "backup archive could not be written"}
		}
		if _, err := writer.Write(member.data); err != nil {
			_ = zipWriter.Close()
			_ = tmp.Close()
			return &domain.Error{Code: domain.ErrUnavailable, Message: "backup archive could not be written"}
		}
	}
	if err := zipWriter.Close(); err != nil {
		_ = tmp.Close()
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup archive could not be closed"}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup archive could not be flushed"}
	}
	if err := tmp.Close(); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup archive could not be closed"}
	}
	if _, err := ReadPackage(tmpName); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup file could not be replaced"}
	}
	cleanup = false
	_ = syncDir(dir)
	return nil
}

func ReadPackage(path string) (Package, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive could not be read"}
	}
	defer reader.Close()
	if err := rejectUnsafeMembers(reader.File); err != nil {
		return Package{}, err
	}
	contents := map[string][]byte{}
	var total int64
	for _, file := range reader.File {
		name := filepath.ToSlash(file.Name)
		if file.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive contains a directory"}
		}
		if file.UncompressedSize64 > MaxMemberBytes {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup member exceeds the size limit"}
		}
		total += int64(file.UncompressedSize64)
		if total > MaxTotalBytes {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive exceeds the size limit"}
		}
		opened, err := file.Open()
		if err != nil {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup member could not be read"}
		}
		limited := io.LimitReader(opened, MaxMemberBytes+1)
		data, err := io.ReadAll(limited)
		_ = opened.Close()
		if err != nil {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup member could not be read"}
		}
		if int64(len(data)) > MaxMemberBytes {
			return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup member exceeds the size limit"}
		}
		contents[name] = data
	}
	required := []string{MemberManifest, MemberDatabase, MemberSettings}
	for _, name := range required {
		if _, ok := contents[name]; !ok {
			return Package{}, &domain.Error{Code: domain.ErrBackupMissingMember, Message: "backup archive is missing a required file"}
		}
	}
	if len(contents) != 3 {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive contains unexpected files"}
	}
	if int64(len(contents[MemberManifest])) > MaxManifestSize {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup manifest exceeds the size limit"}
	}
	if int64(len(contents[MemberSettings])) > MaxSettingsSize {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup settings exceed the size limit"}
	}
	var manifest Manifest
	if err := json.Unmarshal(contents[MemberManifest], &manifest); err != nil {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup manifest could not be parsed"}
	}
	if manifest.FormatVersion != FormatVersion {
		return Package{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup format is not supported"}
	}
	if manifest.SchemaVersion != sqlite.CurrentSchemaVersion {
		return Package{}, &domain.Error{Code: domain.ErrBackupSchemaUnsupported, Message: "backup schema is not supported"}
	}
	if err := verifyMember(manifest, MemberDatabase, contents[MemberDatabase]); err != nil {
		return Package{}, err
	}
	if err := verifyMember(manifest, MemberSettings, contents[MemberSettings]); err != nil {
		return Package{}, err
	}
	return Package{Manifest: manifest, Database: contents[MemberDatabase], Settings: contents[MemberSettings]}, nil
}

func verifyMember(manifest Manifest, name string, data []byte) error {
	meta, ok := manifest.Members[name]
	if !ok {
		return &domain.Error{Code: domain.ErrBackupMissingMember, Message: "backup manifest is missing a member checksum"}
	}
	if meta.SizeBytes != int64(len(data)) {
		return &domain.Error{Code: domain.ErrBackupChecksumFailed, Message: "backup member size does not match the manifest"}
	}
	if !strings.EqualFold(meta.SHA256, sha256Hex(data)) {
		return &domain.Error{Code: domain.ErrBackupChecksumFailed, Message: "backup member checksum does not match the manifest"}
	}
	return nil
}

func rejectUnsafeMembers(files []*zip.File) error {
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if name != MemberManifest && name != MemberDatabase && name != MemberSettings {
			return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive contains an unexpected file"}
		}
		if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
			return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive contains an unsafe path"}
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup archive contains a symbolic link"}
		}
	}
	return nil
}

func DefaultBackupFileName(now time.Time) string {
	return fmt.Sprintf("Nestworth Backup %s%s", now.Local().Format("2006-01-02 15-04"), FileExt)
}

func ExtractDatabase(pkg Package, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore directory could not be created"}
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".nestworth-restore-*.tmp")
	if err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging file could not be created"}
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging permissions could not be set"}
	}
	if _, err := tmp.Write(pkg.Database); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging file could not be written"}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging file could not be flushed"}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging file could not be closed"}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging file could not be installed"}
	}
	_ = syncDir(filepath.Dir(dest))
	return nil
}

func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func WriteAtomicFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nestworth-sidecar-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	_ = syncDir(filepath.Dir(path))
	return nil
}
