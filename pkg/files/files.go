package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"io"
)

type FileInfo struct {
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	Ext      string `json:"ext"`
	Month    string `json:"month,omitempty"`
}

// ListFiles scans AccountDir/msg/file for native unencrypted WeChat attachments.
func ListFiles(accountDir, keyword, ext string, limit, offset int) ([]FileInfo, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	fileRoot := filepath.Join(accountDir, "msg", "file")
	if fi, err := os.Stat(fileRoot); err != nil || !fi.IsDir() {
		return nil, false, fmt.Errorf("file directory not found: %s", fileRoot)
	}

	monthDirs, err := os.ReadDir(fileRoot)
	if err != nil {
		return nil, false, err
	}

	var allFiles []FileInfo
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	for _, md := range monthDirs {
		if !md.IsDir() {
			continue
		}
		monthPath := filepath.Join(fileRoot, md.Name())
		entries, err := os.ReadDir(monthPath)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}

			name := e.Name()
			fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))

			if ext != "" && fileExt != ext {
				continue
			}

			if keyword != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(keyword)) {
				continue
			}

			info, err := e.Info()
			if err != nil {
				continue
			}

			allFiles = append(allFiles, FileInfo{
				FileName: name,
				FilePath: filepath.Join(monthPath, name),
				Size:     info.Size(),
				ModTime:  info.ModTime().Unix(),
				Ext:      fileExt,
				Month:    md.Name(),
			})
		}
	}

	// Sort by modification time descending
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].ModTime > allFiles[j].ModTime
	})

	start := offset
	if start > len(allFiles) {
		start = len(allFiles)
	}
	end := start + limit
	if end > len(allFiles) {
		end = len(allFiles)
	}

	hasMore := end < len(allFiles)
	return allFiles[start:end], hasMore, nil
}

// GetFile locates a specific file by exact or partial name.
func GetFile(accountDir, fileName string) (*FileInfo, error) {
	fileRoot := filepath.Join(accountDir, "msg", "file")
	if fi, err := os.Stat(fileRoot); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("file directory not found: %s", fileRoot)
	}

	monthDirs, err := os.ReadDir(fileRoot)
	if err != nil {
		return nil, err
	}

	cleanTarget := strings.ToLower(strings.TrimSpace(fileName))

	var fallbackMatch *FileInfo

	// Search newest months first
	sort.Slice(monthDirs, func(i, j int) bool {
		return monthDirs[i].Name() > monthDirs[j].Name()
	})

	for _, md := range monthDirs {
		if !md.IsDir() {
			continue
		}
		monthPath := filepath.Join(fileRoot, md.Name())
		entries, err := os.ReadDir(monthPath)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lowerName := strings.ToLower(name)

			if lowerName == cleanTarget {
				info, _ := e.Info()
				return &FileInfo{
					FileName: name,
					FilePath: filepath.Join(monthPath, name),
					Size:     info.Size(),
					ModTime:  info.ModTime().Unix(),
					Ext:      strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")),
					Month:    md.Name(),
				}, nil
			}

			if fallbackMatch == nil && strings.Contains(lowerName, cleanTarget) {
				info, _ := e.Info()
				fallbackMatch = &FileInfo{
					FileName: name,
					FilePath: filepath.Join(monthPath, name),
					Size:     info.Size(),
					ModTime:  info.ModTime().Unix(),
					Ext:      strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")),
					Month:    md.Name(),
				}
			}
		}
	}

	if fallbackMatch != nil {
		return fallbackMatch, nil
	}

	return nil, fmt.Errorf("file not found: %s", fileName)
}

type SyncResult struct {
	Mode         string   `json:"mode"`
	TargetDir    string   `json:"targetDir"`
	TotalFiles   int      `json:"totalFiles"`
	SyncedFiles  int      `json:"syncedFiles"`
	SkippedFiles int      `json:"skippedFiles"`
	SyncedList   []string `json:"syncedList,omitempty"`
	Message      string   `json:"message"`
}

// SyncToGdrive copies WeChat received files or a specified file to Google Drive.
func SyncToGdrive(accountDir, targetDir, filePath string, overwrite bool) (*SyncResult, error) {
	home, _ := os.UserHomeDir()
	if targetDir == "" {
		targetDir = filepath.Join(home, "gdrive", "wechat")
	} else if strings.HasPrefix(targetDir, "~/") {
		targetDir = filepath.Join(home, strings.TrimPrefix(targetDir, "~/"))
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	// 1. Single file upload mode
	if filePath != "" {
		cleanPath := filePath
		if strings.HasPrefix(cleanPath, "~/") {
			cleanPath = filepath.Join(home, strings.TrimPrefix(cleanPath, "~/"))
		}

		// If path doesn't exist directly, try resolving it by file name in accountDir
		if fi, err := os.Stat(cleanPath); err != nil || fi.IsDir() {
			matched, err := GetFile(accountDir, cleanPath)
			if err != nil || matched == nil {
				return nil, fmt.Errorf("file not found: %s", filePath)
			}
			cleanPath = matched.FilePath
		}

		srcFi, err := os.Stat(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read file %s: %w", cleanPath, err)
		}

		destFile := filepath.Join(targetDir, srcFi.Name())
		if destFi, err := os.Stat(destFile); err == nil && !overwrite {
			if destFi.Size() == srcFi.Size() {
				return &SyncResult{
					Mode:         "single_file",
					TargetDir:    targetDir,
					TotalFiles:   1,
					SyncedFiles:  0,
					SkippedFiles: 1,
					Message:      fmt.Sprintf("File already exists in %s with matching size, skipped (set overwrite=true to force)", destFile),
				}, nil
			}
		}

		if err := copyFile(cleanPath, destFile); err != nil {
			return nil, fmt.Errorf("failed to copy file: %w", err)
		}

		return &SyncResult{
			Mode:         "single_file",
			TargetDir:    targetDir,
			TotalFiles:   1,
			SyncedFiles:  1,
			SkippedFiles: 0,
			SyncedList:   []string{srcFi.Name()},
			Message:      fmt.Sprintf("Successfully uploaded %s to %s", srcFi.Name(), destFile),
		}, nil
	}

	// 2. Full sync mode: sync all files from msg/file to targetDir
	fileRoot := filepath.Join(accountDir, "msg", "file")
	if fi, err := os.Stat(fileRoot); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("wechat file directory not found: %s", fileRoot)
	}

	monthDirs, err := os.ReadDir(fileRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read file root %s: %w", fileRoot, err)
	}

	result := &SyncResult{
		Mode:      "full_sync",
		TargetDir: targetDir,
	}

	for _, md := range monthDirs {
		if !md.IsDir() {
			continue
		}
		monthName := md.Name()
		monthSrcDir := filepath.Join(fileRoot, monthName)
		monthDestDir := filepath.Join(targetDir, monthName)

		entries, err := os.ReadDir(monthSrcDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}

			result.TotalFiles++
			srcPath := filepath.Join(monthSrcDir, e.Name())
			destPath := filepath.Join(monthDestDir, e.Name())

			srcInfo, err := e.Info()
			if err != nil {
				continue
			}

			if destInfo, err := os.Stat(destPath); err == nil && !overwrite {
				if destInfo.Size() == srcInfo.Size() {
					result.SkippedFiles++
					continue
				}
			}

			_ = os.MkdirAll(monthDestDir, 0755)
			if err := copyFile(srcPath, destPath); err == nil {
				result.SyncedFiles++
				result.SyncedList = append(result.SyncedList, filepath.Join(monthName, e.Name()))
			}
		}
	}

	result.Message = fmt.Sprintf("Sync completed: %d files uploaded, %d skipped (up to date) to %s",
		result.SyncedFiles, result.SkippedFiles, targetDir)
	return result, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
