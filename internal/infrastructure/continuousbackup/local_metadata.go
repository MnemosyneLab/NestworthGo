package continuousbackup

import (
	"os"
	"path/filepath"
	"strings"
)

// removeStreamMetadata reclaims only the current process's sealed stream.
// It never enumerates old streams or changes the remote retention ledger.
func (m *Manager) removeStreamMetadata(path string) error {
	stream := m.identity.StreamID
	if !validStream(stream) || m.replicaConfig.BackupID != strings.Split(stream, "/")[0] {
		return ErrUnavailable
	}
	parts := strings.Split(stream, "/")
	if path != filepath.Join(filepath.Dir(m.path), ".nestworth-replication", parts[0], parts[1]) {
		return ErrUnavailable
	}
	base, err := os.OpenRoot(filepath.Dir(m.path))
	if err != nil {
		return err
	}
	defer base.Close()
	metadata, err := openMetadataDirectory(base, ".nestworth-replication")
	if err != nil {
		return err
	}
	defer metadata.Close()
	group, err := openMetadataDirectory(metadata, parts[0])
	if err != nil {
		return err
	}
	defer group.Close()
	info, err := group.Lstat(parts[1])
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnavailable
	}
	// Root pins the parent; RemoveAll does not follow the leaf or child links.
	// Replacing an ancestor cannot redirect deletion to a sibling or remote.
	return group.RemoveAll(parts[1])
}

func openMetadataDirectory(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnavailable
	}
	dir, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := dir.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		dir.Close()
		return nil, ErrUnavailable
	}
	return dir, nil
}
