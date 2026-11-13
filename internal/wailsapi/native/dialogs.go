package native

// Dialog text identifiers are resolved by the desktop platform at display time.
const (
	BackupTitle  = "backup"
	ExportTitle  = "export"
	RestoreTitle = "restore"
	BackupFilter = "backup_filter"
	ExportFilter = "export_filter"
)

// Dialogs is the platform file-picker surface. Empty path means the user
// cancelled; that is not an error.
type Dialogs interface {
	SaveFile(title, filename, filterName, pattern string) (string, error)
	OpenFile(title, filterName, pattern string) (string, error)
	ConfirmReplace(fileName string) (bool, error)
}

type Quitter interface {
	Quit()
}

type RefreshGate interface {
	CancelAllAndWait()
}

type NoopDialogs struct{}

func (NoopDialogs) SaveFile(string, string, string, string) (string, error) { return "", nil }
func (NoopDialogs) OpenFile(string, string, string) (string, error)         { return "", nil }
func (NoopDialogs) ConfirmReplace(string) (bool, error)                     { return false, nil }

type NoopQuitter struct{}

func (NoopQuitter) Quit() {}

type NoopRefresh struct{}

func (NoopRefresh) CancelAllAndWait() {}
