package sqlite

// OpenReviewPerformanceDatabase installs a test metrics driver from creation.
// Replacing a live pool would correctly lose private genesis provenance.
// The internal test suites supply metrics or fault decorators from creation.
// Desktop initialization always uses Open with the managed SQLite driver.
func OpenReviewPerformanceDatabase(path, driverName string) (*DB, error) {
	return openDatabase(path, driverName)
}
