//go:build perf

package sqlite

// OpenReviewPerformanceDatabase installs a test metrics driver from creation.
// Replacing a live pool would correctly lose private genesis provenance.
// This adapter is absent from normal builds; the application supplies its
// synthetic metrics decorator only in perf-tagged tests.
func OpenReviewPerformanceDatabase(path, driverName string) (*DB, error) {
	return openDatabase(path, driverName)
}
