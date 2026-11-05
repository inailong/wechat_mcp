//go:build (!linux && !windows) || (linux && !cgo)

package wcdb

func findNativeLibPlatform() string {
	return ""
}

func initLocked(libPath string) error {
	return ErrNotSupported
}

func openAccountLocked(sessionDbPath, hexKey string) (int64, error) {
	return 0, ErrNotSupported
}

func getSessionsLocked() ([]map[string]interface{}, error) {
	return nil, ErrNotSupported
}

func getMessagesLocked(username string, limit, offset int) ([]map[string]interface{}, error) {
	return nil, ErrNotSupported
}

func searchMessagesLocked(sessionId, keyword string, limit, offset, beginTs, endTs int) ([]map[string]interface{}, error) {
	return nil, ErrNotSupported
}

func getContactLocked(username string) (map[string]interface{}, error) {
	return nil, ErrNotSupported
}

func execQueryLocked(kind, target, sql string) (string, error) {
	return "", ErrNotSupported
}

func closeLocked() {
}
