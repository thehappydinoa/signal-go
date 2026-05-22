package store

// ImportedContact is a contact entry restored from a message backup.
type ImportedContact struct {
	ACI        string
	PNI        string
	E164       string
	ProfileKey []byte
	GivenName  string
	FamilyName string
	Blocked    bool
}

// ImportedGroup is a Groups v2 entry restored from a message backup.
type ImportedGroup struct {
	MasterKey []byte // 32 bytes
	Title     string
	Blocked   bool
}

// BackupImportStore persists contact and group list entries imported from
// transfer archives. Implementations include [sqlstore.DB].
type BackupImportStore interface {
	SaveImportedContact(c ImportedContact) error
	SaveImportedGroup(g ImportedGroup) error
	LoadImportedContacts() ([]ImportedContact, error)
	LoadImportedGroups() ([]ImportedGroup, error)
}
