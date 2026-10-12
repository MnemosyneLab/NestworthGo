package continuousbackup

import "github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"

// NewSyntheticCloudManager is compiled only for this package's external
// integration tests. It runs the real manager against disposable local objects.
func NewSyntheticCloudManager(path string, db *sqlite.DB, root string) (*Manager, error) {
	manager, err := New(path, db)
	if err != nil {
		return nil, err
	}
	manager.factory = func(config) backend { return fileBackend{root: root} }
	return manager, nil
}
