package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const freshnessWindow = 72 * time.Hour

// ResolveOutputPath returns the final file path for collection output.
func ResolveOutputPath(base string) string {
	if strings.TrimSpace(base) == "" {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, "collection.json")
	}

	clean := filepath.Clean(base)
	if strings.HasSuffix(base, string(os.PathSeparator)) {
		return filepath.Join(clean, "collection.json")
	}
	if stat, err := os.Stat(clean); err == nil && stat.IsDir() {
		return filepath.Join(clean, "collection.json")
	}
	return clean
}

// EnforceFreshness blocks writing over recent output files unless force is enabled.
func EnforceFreshness(outPath string, force bool) error {
	st, err := os.Stat(outPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("unable to read existing output file metadata: %w", err)
	}

	if time.Since(st.ModTime()) < freshnessWindow && !force {
		return fmt.Errorf("output file is too recent: %s. Use --force or MCD_FORCE=1 to override", outPath)
	}
	return nil
}

// WriteJSONFile writes payload as pretty JSON to outPath, creating parent directories.
func WriteJSONFile(outPath string, payload any) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".mcd-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(blob); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		if removeErr := os.Remove(outPath); removeErr == nil {
			if secondErr := os.Rename(tmpPath, outPath); secondErr == nil {
				cleanup = false
				return nil
			} else {
				return secondErr
			}
		}
		return err
	}
	cleanup = false
	return nil
}
