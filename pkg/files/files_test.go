package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
	accountDir := filepath.Join(home, "Documents", "xwechat_files", "wxid_n9x0u8sjzc7f22_c6a1")

	files, hasMore, err := ListFiles(accountDir, "", "", 10, 0)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	t.Logf("ListFiles returned %d files (hasMore: %v)", len(files), hasMore)
	if len(files) == 0 {
		t.Fatalf("Expected files > 0")
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
		t.Fatalf("ListFiles pdf failed: %v", err)
	}
	t.Logf("Found %d PDF files", len(pdfFiles))
	for _, p := range pdfFiles {
		t.Logf("  PDF: %s", p.FileName)
	}
}
