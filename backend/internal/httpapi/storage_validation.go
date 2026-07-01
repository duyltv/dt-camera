package httpapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type storageValidationResult struct {
	Valid                 bool    `json:"valid"`
	Exists                bool    `json:"exists"`
	Writable              bool    `json:"writable"`
	TotalBytes            uint64  `json:"total_bytes"`
	FreeBytes             uint64  `json:"free_bytes"`
	UsedBytes             uint64  `json:"used_bytes"`
	UsedPercent           float64 `json:"used_percent"`
	Message               string  `json:"message,omitempty"`
	LatestValidationError string  `json:"latest_validation_error,omitempty"`
}

func validateStoragePath(containerPath string) storageValidationResult {
	if containerPath == "" {
		return storageValidationResult{Valid: false, Message: "container_path is required"}
	}

	result := storageValidationResult{}
	info, err := os.Stat(containerPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Message = "container_path does not exist inside the backend container"
			result.LatestValidationError = result.Message
			return result
		}
		result.Message = fmt.Sprintf("container_path cannot be inspected: %v", err)
		result.LatestValidationError = result.Message
		return result
	}
	result.Exists = true
	fillDiskStats(containerPath, &result)

	if !info.IsDir() {
		result.Message = "container_path exists but is not a directory"
		result.LatestValidationError = result.Message
		return result
	}

	tempFile, err := os.CreateTemp(containerPath, ".dt-camera-write-test-*")
	if err != nil {
		result.Message = fmt.Sprintf("container_path is not writable: %v", err)
		result.LatestValidationError = result.Message
		return result
	}

	name := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(name)
		result.Message = fmt.Sprintf("container_path write test could not close temporary file: %v", err)
		result.LatestValidationError = result.Message
		return result
	}

	if err := os.Remove(name); err != nil {
		result.Message = fmt.Sprintf("container_path write test could not remove temporary file: %v", err)
		result.LatestValidationError = result.Message
		return result
	}

	result.Valid = true
	result.Writable = true
	return result
}

func fillDiskStats(path string, result *storageValidationResult) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}

	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bavail * blockSize
	used := total - free
	usedPercent := 0.0
	if total > 0 {
		usedPercent = (float64(used) / float64(total)) * 100
	}
	result.TotalBytes = total
	result.FreeBytes = free
	result.UsedBytes = used
	result.UsedPercent = usedPercent
}

func cleanStoragePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}
