package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// TemporaryFiles owns processing-only files. It never exposes paths to HTTP clients.
type TemporaryFiles struct{ Root string }

func NewTemporaryFiles() TemporaryFiles {
	root := os.Getenv("INGESTION_TMP_DIR")
	if root == "" {
		root = filepath.Join(os.TempDir(), "homework-canvas-ingestion")
	}
	return TemporaryFiles{Root: root}
}
func (t TemporaryFiles) JobDir(id uuid.UUID) string { return filepath.Join(t.Root, id.String()) }
func (t TemporaryFiles) CreateJobDir(id uuid.UUID) (string, error) {
	dir := t.JobDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
func (t TemporaryFiles) SourcePath(jobID uuid.UUID, filename string) (string, error) {
	name := filepath.Base(filename)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "", fmt.Errorf("invalid filename")
	}
	ext := strings.ToLower(filepath.Ext(name))
	return filepath.Join(t.JobDir(jobID), "source"+ext), nil
}
func (t TemporaryFiles) RemoveJob(id uuid.UUID) error { return os.RemoveAll(t.JobDir(id)) }
