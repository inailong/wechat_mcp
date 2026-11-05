//go:build linux && cgo

package wcdb

/*
#cgo LDFLAGS: -ldl -Wl,--export-dynamic
#define _GNU_SOURCE
#include <stdint.h>
#include <stdlib.h>
#include <time.h>
#include <dlfcn.h>
#include <unistd.h>
#include <string.h>
#include <stdio.h>

static time_t (*real_time)(time_t *tloc) = NULL;
static int (*real_clock_gettime)(clockid_t clk_id, struct timespec *tp) = NULL;
static ssize_t (*real_readlink)(const char *pathname, char *buf, size_t bufsiz) = NULL;

time_t time(time_t *tloc) {
    time_t t = 1790000000;
    if (tloc) *tloc = t;
    return t;
}

int clock_gettime(clockid_t clk_id, struct timespec *tp) {
    if (clk_id == CLOCK_REALTIME) {
        tp->tv_sec = 1790000000;
        tp->tv_nsec = 0;
        return 0;
    }
    if (!real_clock_gettime) real_clock_gettime = dlsym(RTLD_NEXT, "clock_gettime");
    return real_clock_gettime(clk_id, tp);
}

ssize_t readlink(const char *pathname, char *buf, size_t bufsiz) {
    if (!real_readlink) real_readlink = dlsym(RTLD_NEXT, "readlink");
    if (pathname && strcmp(pathname, "/proc/self/exe") == 0) {
        const char *fake = "/usr/bin/weflow";
        size_t len = strlen(fake);
        if (bufsiz < len) len = bufsiz;
        memcpy(buf, fake, len);
        return len;
    }
    return real_readlink(pathname, buf, bufsiz);
}

typedef int32_t (*fn_init_protection)(const char* resourcePath);
typedef int32_t (*fn_wcdb_init)();
typedef int32_t (*fn_wcdb_shutdown)();
typedef int32_t (*fn_wcdb_open_account)(const char* path, const char* key, int64_t* handle);
typedef int32_t (*fn_wcdb_close_account)(int64_t handle);
typedef int32_t (*fn_wcdb_get_sessions)(int64_t handle, char** outJson);
typedef int32_t (*fn_wcdb_get_messages)(int64_t handle, const char* username, int32_t limit, int32_t offset, char** outJson);
typedef int32_t (*fn_wcdb_search_messages)(int64_t handle, const char* sessionId, const char* keyword, int32_t limit, int32_t offset, int32_t beginTimestamp, int32_t endTimestamp, char** outJson);
typedef int32_t (*fn_wcdb_get_contact)(int64_t handle, const char* username, char** outJson);
typedef int32_t (*fn_wcdb_exec_query)(int64_t handle, const char* kind, const char* target, const char* sql, char** outJson);
typedef void (*fn_wcdb_free_string)(char* str);

static void* g_lib = NULL;
static fn_init_protection g_protection = NULL;
static fn_wcdb_init g_init = NULL;
static fn_wcdb_shutdown g_shutdown = NULL;
static fn_wcdb_open_account g_open = NULL;
static fn_wcdb_close_account g_close = NULL;
static fn_wcdb_get_sessions g_sessions = NULL;
static fn_wcdb_get_messages g_messages = NULL;
static fn_wcdb_search_messages g_search = NULL;
static fn_wcdb_get_contact g_contact = NULL;
static fn_wcdb_exec_query g_exec_query = NULL;
static fn_wcdb_free_string g_free = NULL;

static int load_wcdb_lib(const char* path) {
    if (g_lib) return 0;
    g_lib = dlopen(path, RTLD_LAZY | RTLD_GLOBAL);
    if (!g_lib) return -1;

    g_protection = (fn_init_protection)dlsym(g_lib, "InitProtection");
    g_init = (fn_wcdb_init)dlsym(g_lib, "wcdb_init");
    g_shutdown = (fn_wcdb_shutdown)dlsym(g_lib, "wcdb_shutdown");
    g_open = (fn_wcdb_open_account)dlsym(g_lib, "wcdb_open_account");
    g_close = (fn_wcdb_close_account)dlsym(g_lib, "wcdb_close_account");
    g_sessions = (fn_wcdb_get_sessions)dlsym(g_lib, "wcdb_get_sessions");
    g_messages = (fn_wcdb_get_messages)dlsym(g_lib, "wcdb_get_messages");
    g_search = (fn_wcdb_search_messages)dlsym(g_lib, "wcdb_search_messages");
    g_contact = (fn_wcdb_get_contact)dlsym(g_lib, "wcdb_get_contact");
    g_exec_query = (fn_wcdb_exec_query)dlsym(g_lib, "wcdb_exec_query");
    g_free = (fn_wcdb_free_string)dlsym(g_lib, "wcdb_free_string");
    return 0;
}

static int c_protection(const char* p) { return g_protection ? g_protection(p) : -1; }
static int c_init() { return g_init ? g_init() : -1; }
static int c_shutdown() { return g_shutdown ? g_shutdown() : -1; }
static int c_open(const char* p, const char* k, int64_t* h) { return g_open ? g_open(p, k, h) : -1; }
static int c_close(int64_t h) { return g_close ? g_close(h) : -1; }
static int c_sessions(int64_t h, char** out) { return g_sessions ? g_sessions(h, out) : -1; }
static int c_messages(int64_t h, const char* u, int32_t lim, int32_t off, char** out) {
    return g_messages ? g_messages(h, u, lim, off, out) : -1;
}
static int c_search(int64_t h, const char* sid, const char* kw, int32_t lim, int32_t off, int32_t bts, int32_t ets, char** out) {
    return g_search ? g_search(h, sid, kw, lim, off, bts, ets, out) : -1;
}
static int c_contact(int64_t h, const char* u, char** out) { return g_contact ? g_contact(h, u, out) : -1; }
static int c_exec_query(int64_t h, const char* kind, const char* target, const char* sql, char** out) {
    return g_exec_query ? g_exec_query(h, kind, target, sql, out) : -1;
}
static void c_free(char* s) { if (g_free && s) g_free(s); }
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

func findNativeLibPlatform() string {
	candidates := []string{
		"native/linux/x64/libwcdb_api.so",
		"../native/linux/x64/libwcdb_api.so",
		"/home/sesley/Workspace/wechat_mcp/native/linux/x64/libwcdb_api.so",
		"/home/sesley/Workspace/WeFlow/resources/wcdb/linux/x64/libwcdb_api.so",
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
		return errors.New("libwcdb_api.so not found")
	}

	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	if rc := C.load_wcdb_lib(cLibPath); rc != 0 {
		return fmt.Errorf("failed to dlopen %s", libPath)
	}

	resDir := filepath.Dir(libPath)
	cResDir := C.CString(resDir)
	defer C.free(unsafe.Pointer(cResDir))

	pRc := C.c_protection(cResDir)
	if pRc != 0 {
		return fmt.Errorf("InitProtection failed: %d", pRc)
	}

	iRc := C.c_init()
	if iRc != 0 {
		return fmt.Errorf("wcdb_init failed: %d", iRc)
	}

	initialized = true
	return nil
}

func openAccountLocked(sessionDbPath, hexKey string) (int64, error) {
	cPath := C.CString(sessionDbPath)
	cKey := C.CString(hexKey)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cKey))

	var handle C.int64_t
	rc := C.c_open(cPath, cKey, &handle)
	if rc != 0 || handle <= 0 {
		return 0, fmt.Errorf("wcdb_open_account failed with code %d", rc)
	}

	activeHandle = int64(handle)
	return activeHandle, nil
}

func getSessionsLocked() ([]map[string]interface{}, error) {
	var outJson *C.char
	rc := C.c_sessions(C.int64_t(activeHandle), &outJson)
	if rc != 0 || outJson == nil {
		return nil, fmt.Errorf("wcdb_get_sessions failed: %d", rc)
	}
	defer C.c_free(outJson)

	str := C.GoString(outJson)
	var sessions []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse sessions JSON: %w", err)
	}
	return sessions, nil
}

func getMessagesLocked(username string, limit, offset int) ([]map[string]interface{}, error) {
	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))

	var outJson *C.char
	rc := C.c_messages(C.int64_t(activeHandle), cUser, C.int32_t(limit), C.int32_t(offset), &outJson)
	if rc != 0 || outJson == nil {
		return nil, fmt.Errorf("wcdb_get_messages failed: %d", rc)
	}
	defer C.c_free(outJson)

	str := C.GoString(outJson)
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse messages JSON: %w", err)
	}
	return messages, nil
}

func searchMessagesLocked(sessionId, keyword string, limit, offset, beginTs, endTs int) ([]map[string]interface{}, error) {
	cSid := C.CString(sessionId)
	cKw := C.CString(keyword)
	defer C.free(unsafe.Pointer(cSid))
	defer C.free(unsafe.Pointer(cKw))

	var outJson *C.char
	rc := C.c_search(C.int64_t(activeHandle), cSid, cKw, C.int32_t(limit), C.int32_t(offset), C.int32_t(beginTs), C.int32_t(endTs), &outJson)
	if rc != 0 || outJson == nil {
		return nil, fmt.Errorf("wcdb_search_messages failed: %d", rc)
	}
	defer C.c_free(outJson)

	str := C.GoString(outJson)
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(str), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse search messages JSON: %w", err)
	}
	return messages, nil
}

func getContactLocked(username string) (map[string]interface{}, error) {
	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))

	var outJson *C.char
	rc := C.c_contact(C.int64_t(activeHandle), cUser, &outJson)
	if rc != 0 || outJson == nil {
		return nil, fmt.Errorf("wcdb_get_contact failed: %d", rc)
	}
	defer C.c_free(outJson)

	str := C.GoString(outJson)
	var contact map[string]interface{}
	if err := json.Unmarshal([]byte(str), &contact); err != nil {
		return nil, fmt.Errorf("failed to parse contact JSON: %w", err)
	}
	return contact, nil
}

func execQueryLocked(kind, target, sql string) (string, error) {
	cKind := C.CString(kind)
	cTarget := C.CString(target)
	cSql := C.CString(sql)
	defer C.free(unsafe.Pointer(cKind))
	defer C.free(unsafe.Pointer(cTarget))
	defer C.free(unsafe.Pointer(cSql))

	var outJson *C.char
	rc := C.c_exec_query(C.int64_t(activeHandle), cKind, cTarget, cSql, &outJson)
	if rc != 0 || outJson == nil {
		return "", fmt.Errorf("wcdb_exec_query failed: %d", rc)
	}
	defer C.c_free(outJson)

	return C.GoString(outJson), nil
}

func closeLocked() {
	C.c_shutdown()
}
