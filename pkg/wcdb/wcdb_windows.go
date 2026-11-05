//go:build windows

package wcdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	hLib        syscall.Handle
	pProtection uintptr
	pInit       uintptr
	pShutdown   uintptr
	pOpen       uintptr
	pClose      uintptr
	pSessions   uintptr
	pMessages   uintptr
	pSearch     uintptr
	pContact    uintptr
	pExecQuery  uintptr
	pFree       uintptr
)

func findNativeLibPlatform() string {
	appData := os.Getenv("APPDATA")
	localAppData := os.Getenv("LOCALAPPDATA")
	userProfile := os.Getenv("USERPROFILE")

	candidates := []string{
		"native/windows/x64/wcdb_api.dll",
		"../native/windows/x64/wcdb_api.dll",
		"wcdb_api.dll",
		filepath.Join(userProfile, "Workspace", "wechat_mcp", "native", "windows", "x64", "wcdb_api.dll"),
		filepath.Join(localAppData, "Programs", "WeFlow", "resources", "wcdb", "win32", "x64", "wcdb_api.dll"),
		filepath.Join(appData, "WeFlow", "resources", "wcdb", "win32", "x64", "wcdb_api.dll"),
		filepath.Join("C:\\Program Files", "WeFlow", "resources", "wcdb", "win32", "x64", "wcdb_api.dll"),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Size() > 0 {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func initLocked(libPath string) error {
	if initialized {
		return nil
	}

	if libPath == "" {
		libPath = findNativeLibPlatform()
	}
	if libPath == "" {
		return errors.New("wcdb_api.dll not found")
	}

	h, err := syscall.LoadLibrary(libPath)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", libPath, err)
	}
	hLib = h

	pProtection, _ = syscall.GetProcAddress(hLib, "InitProtection")
	pInit, _ = syscall.GetProcAddress(hLib, "wcdb_init")
	pShutdown, _ = syscall.GetProcAddress(hLib, "wcdb_shutdown")
	pOpen, _ = syscall.GetProcAddress(hLib, "wcdb_open_account")
	pClose, _ = syscall.GetProcAddress(hLib, "wcdb_close_account")
	pSessions, _ = syscall.GetProcAddress(hLib, "wcdb_get_sessions")
	pMessages, _ = syscall.GetProcAddress(hLib, "wcdb_get_messages")
	pSearch, _ = syscall.GetProcAddress(hLib, "wcdb_search_messages")
	pContact, _ = syscall.GetProcAddress(hLib, "wcdb_get_contact")
	pExecQuery, _ = syscall.GetProcAddress(hLib, "wcdb_exec_query")
	pFree, _ = syscall.GetProcAddress(hLib, "wcdb_free_string")

	resDir := filepath.Dir(libPath)
	resDirPtr, err := syscall.BytePtrFromString(resDir)
	if err == nil && pProtection != 0 {
		r, _, _ := syscall.SyscallN(pProtection, uintptr(unsafe.Pointer(resDirPtr)))
		if int32(r) != 0 {
			return fmt.Errorf("InitProtection failed: %d", int32(r))
		}
	}

	if pInit != 0 {
		r, _, _ := syscall.SyscallN(pInit)
		if int32(r) != 0 {
			return fmt.Errorf("wcdb_init failed: %d", int32(r))
		}
	}

	initialized = true
	return nil
}

func openAccountLocked(sessionDbPath, hexKey string) (int64, error) {
	if pOpen == 0 {
		return 0, errors.New("wcdb_open_account not found in DLL")
	}

	pPath, err := syscall.BytePtrFromString(sessionDbPath)
	if err != nil {
		return 0, err
	}
	pKey, err := syscall.BytePtrFromString(hexKey)
	if err != nil {
		return 0, err
	}

	var handle int64
	r, _, _ := syscall.SyscallN(pOpen, uintptr(unsafe.Pointer(pPath)), uintptr(unsafe.Pointer(pKey)), uintptr(unsafe.Pointer(&handle)))
	if int32(r) != 0 || handle <= 0 {
		return 0, fmt.Errorf("wcdb_open_account failed with code %d", int32(r))
	}

	activeHandle = handle
	return activeHandle, nil
}

func getCString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var b []byte
	p := (*byte)(unsafe.Pointer(ptr))
	for *p != 0 {
		b = append(b, *p)
		ptr++
		p = (*byte)(unsafe.Pointer(ptr))
	}
	return string(b)
}

func freeCString(ptr uintptr) {
	if pFree != 0 && ptr != 0 {
		syscall.SyscallN(pFree, ptr)
	}
}

func getSessionsLocked() ([]map[string]interface{}, error) {
	if pSessions == 0 {
		return nil, errors.New("wcdb_get_sessions not found in DLL")
	}

	var outJsonPtr uintptr
	r, _, _ := syscall.SyscallN(pSessions, uintptr(activeHandle), uintptr(unsafe.Pointer(&outJsonPtr)))
	if int32(r) != 0 || outJsonPtr == 0 {
		return nil, fmt.Errorf("wcdb_get_sessions failed: %d", int32(r))
	}
	defer freeCString(outJsonPtr)

	str := getCString(outJsonPtr)
	var sessions []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse sessions JSON: %w", err)
	}
	return sessions, nil
}

func getMessagesLocked(username string, limit, offset int) ([]map[string]interface{}, error) {
	if pMessages == 0 {
		return nil, errors.New("wcdb_get_messages not found in DLL")
	}

	pUser, err := syscall.BytePtrFromString(username)
	if err != nil {
		return nil, err
	}

	var outJsonPtr uintptr
	r, _, _ := syscall.SyscallN(pMessages, uintptr(activeHandle), uintptr(unsafe.Pointer(pUser)), uintptr(limit), uintptr(offset), uintptr(unsafe.Pointer(&outJsonPtr)))
	if int32(r) != 0 || outJsonPtr == 0 {
		return nil, fmt.Errorf("wcdb_get_messages failed: %d", int32(r))
	}
	defer freeCString(outJsonPtr)

	str := getCString(outJsonPtr)
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse messages JSON: %w", err)
	}
	return messages, nil
}

func searchMessagesLocked(sessionId, keyword string, limit, offset, beginTs, endTs int) ([]map[string]interface{}, error) {
	if pSearch == 0 {
		return nil, errors.New("wcdb_search_messages not found in DLL")
	}

	pSid, err := syscall.BytePtrFromString(sessionId)
	if err != nil {
		return nil, err
	}
	pKw, err := syscall.BytePtrFromString(keyword)
	if err != nil {
		return nil, err
	}

	var outJsonPtr uintptr
	r, _, _ := syscall.SyscallN(pSearch, uintptr(activeHandle), uintptr(unsafe.Pointer(pSid)), uintptr(unsafe.Pointer(pKw)), uintptr(limit), uintptr(offset), uintptr(beginTs), uintptr(endTs), uintptr(unsafe.Pointer(&outJsonPtr)))
	if int32(r) != 0 || outJsonPtr == 0 {
		return nil, fmt.Errorf("wcdb_search_messages failed: %d", int32(r))
	}
	defer freeCString(outJsonPtr)

	str := getCString(outJsonPtr)
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse search messages JSON: %w", err)
	}
	return messages, nil
}

func getContactLocked(username string) (map[string]interface{}, error) {
	if pContact == 0 {
		return nil, errors.New("wcdb_get_contact not found in DLL")
	}

	pUser, err := syscall.BytePtrFromString(username)
	if err != nil {
		return nil, err
	}

	var outJsonPtr uintptr
	r, _, _ := syscall.SyscallN(pContact, uintptr(activeHandle), uintptr(unsafe.Pointer(pUser)), uintptr(unsafe.Pointer(&outJsonPtr)))
	if int32(r) != 0 || outJsonPtr == 0 {
		return nil, fmt.Errorf("wcdb_get_contact failed: %d", int32(r))
	}
	defer freeCString(outJsonPtr)

	str := getCString(outJsonPtr)
	var contact map[string]interface{}
	if err := json.Unmarshal([]byte(str), &contact); err != nil {
		return nil, fmt.Errorf("failed to parse contact JSON: %w", err)
	}
	return contact, nil
}

func execQueryLocked(kind, target, sql string) (string, error) {
	if pExecQuery == 0 {
		return "", errors.New("wcdb_exec_query not found in DLL")
	}

	pKind, _ := syscall.BytePtrFromString(kind)
	pTarget, _ := syscall.BytePtrFromString(target)
	pSql, _ := syscall.BytePtrFromString(sql)

	var outJsonPtr uintptr
	r, _, _ := syscall.SyscallN(pExecQuery, uintptr(activeHandle), uintptr(unsafe.Pointer(pKind)), uintptr(unsafe.Pointer(pTarget)), uintptr(unsafe.Pointer(pSql)), uintptr(unsafe.Pointer(&outJsonPtr)))
	if int32(r) != 0 || outJsonPtr == 0 {
		return "", fmt.Errorf("wcdb_exec_query failed: %d", int32(r))
	}
	defer freeCString(outJsonPtr)

	return getCString(outJsonPtr), nil
}

func closeLocked() {
	if pShutdown != 0 {
		syscall.SyscallN(pShutdown)
	}
	if hLib != 0 {
		syscall.FreeLibrary(hLib)
		hLib = 0
	}
}
