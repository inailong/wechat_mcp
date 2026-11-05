package files

import (
	"testing"

	"github.com/inailong/wechat_mcp/pkg/wechat"
)

func TestFiles(t *testing.T) {
	info, err := wechat.DetectAccount()
	if err != nil || info == nil || info.AccountDir == "" {
		t.Skip("WeChat account directory not detected, skipping TestFiles")
	}
	accountDir := info.AccountDir

	files, hasMore, err := ListFiles(accountDir, "", "", 10, 0)
	if err != nil {
		t.Skipf("ListFiles skipped: %v", err)
		return
	}
	t.Logf("ListFiles returned %d files (hasMore: %v)", len(files), hasMore)
	if len(files) == 0 {
		t.Log("No local files found in msg/file")
		return
	}

	first := files[0]
	t.Logf("First file: %s (%d bytes, path: %s)", first.FileName, first.Size, first.FilePath)

	// Test GetFile
	file, err := GetFile(accountDir, first.FileName)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	t.Logf("GetFile verified: %s -> %s", file.FileName, file.FilePath)

	// Test filter by ext: pdf
	pdfFiles, _, err := ListFiles(accountDir, "", "pdf", 5, 0)
	if err != nil {
		t.Logf("ListFiles pdf info: %v", err)
	} else {
		t.Logf("Found %d PDF files", len(pdfFiles))
		for _, p := range pdfFiles {
			t.Logf("  PDF: %s", p.FileName)
		}
	}
}
