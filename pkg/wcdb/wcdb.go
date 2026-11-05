package wcdb

import (
	"errors"
	"sync"
)

var (
	mu           sync.Mutex
	initialized  bool
	activeHandle int64
)

// ErrNotSupported indicates that native WCDB is unavailable on the current platform/configuration.
var ErrNotSupported = errors.New("native wcdb driver not supported on this platform/configuration (use WeFlow HTTP API fallback)")

// FindNativeLib locates the native library in bundled or standard locations.
func FindNativeLib() string {
	return findNativeLibPlatform()
}

// Init loads the native library and performs security handshake.
func Init(libPath string) error {
	mu.Lock()
	defer mu.Unlock()
	return initLocked(libPath)
}

// OpenAccount opens a user's WeChat databases.
func OpenAccount(sessionDbPath, hexKey string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()

	if !initialized {
		if err := initLocked(""); err != nil {
			return 0, err
		}
	}

	if activeHandle > 0 {
		return activeHandle, nil
	}

	return openAccountLocked(sessionDbPath, hexKey)
}

// GetSessions returns all sessions directly from WCDB.
func GetSessions() ([]map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle <= 0 {
		return nil, errors.New("no active database handle")
	}

	return getSessionsLocked()
}

// GetMessages returns messages for a given talker.
func GetMessages(username string, limit, offset int) ([]map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle <= 0 {
		return nil, errors.New("no active database handle")
	}

	return getMessagesLocked(username, limit, offset)
}

// SearchMessages searches messages by keyword across all sessions or in a given session.
func SearchMessages(sessionId, keyword string, limit, offset, beginTs, endTs int) ([]map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle <= 0 {
		return nil, errors.New("no active database handle")
	}

	return searchMessagesLocked(sessionId, keyword, limit, offset, beginTs, endTs)
}

// GetContact queries contact information for a username.
func GetContact(username string) (map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle <= 0 {
		return nil, errors.New("no active database handle")
	}

	return getContactLocked(username)
}

// ExecQuery executes a raw SQL query against a specific database kind ("session", "message", "contact", etc.)
func ExecQuery(kind, target, sql string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle <= 0 {
		return "", errors.New("no active database handle")
	}

	return execQueryLocked(kind, target, sql)
}

// Close closes the active account and shuts down native WCDB.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if activeHandle > 0 {
		closeLocked()
		activeHandle = 0
		initialized = false
	}
}
