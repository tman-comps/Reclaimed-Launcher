// Package install downloads release assets, verifies them against
// SHA256SUMS, and places them atomically. It only writes beneath the
// directories it is given.
package install

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Progress reports download progress; total is -1 when unknown.
type Progress func(done, total int64)

// ErrChecksum is returned when a download doesn't match SHA256SUMS.
var ErrChecksum = errors.New("checksum mismatch")

// Download fetches url to dest, hashing as it goes, and deletes dest unless
// its SHA-256 equals wantSHA. The response body is capped at maxBytes.
func Download(ctx context.Context, client *http.Client, ua, url, dest, wantSHA string, maxBytes int64, progress Progress) error {
	wantSHA = strings.ToLower(wantSHA)
	if len(wantSHA) != 64 {
		return errors.New("no expected SHA-256 for this file; refusing to download")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return fmt.Errorf("download is larger than the %d MB limit", maxBytes>>20)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(dest)
		}
	}()
	h := sha256.New()
	pr := &progressReader{r: io.LimitReader(resp.Body, maxBytes+1), total: resp.ContentLength, fn: progress}
	n, err := io.Copy(io.MultiWriter(f, h), pr)
	if err != nil {
		return err
	}
	if n > maxBytes {
		return fmt.Errorf("download is larger than the %d MB limit", maxBytes>>20)
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksum, wantSHA[:12], got[:12])
	}
	ok = true
	return f.Close()
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	fn    Progress
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.fn != nil && (time.Since(p.last) > 100*time.Millisecond || err == io.EOF) {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}

// HashFile returns the lowercase hex SHA-256 of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Limits for archive extraction.
const (
	MaxFiles      = 20000
	MaxUnpacked   = 16 << 30
	maxEntryRatio = 1000 // reject zip bombs
)

// Unzip extracts src into destDir (which must not exist yet). If every entry
// sits under one top-level folder, that folder is stripped so destDir holds
// the game files directly.
func Unzip(src, destDir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	if len(zr.File) > MaxFiles {
		return errors.New("archive has too many files")
	}
	prefix := commonTopDir(zr.File)
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("%s already exists", destDir)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	var total uint64
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		name = strings.TrimPrefix(name, prefix)
		if name == "" || name == "/" {
			continue
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(destDir, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains a symlink: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += f.UncompressedSize64
		if total > MaxUnpacked {
			return errors.New("archive unpacks too large")
		}
		if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > maxEntryRatio {
			return fmt.Errorf("suspicious compression ratio for %q", f.Name)
		}
		if err := extractOne(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func commonTopDir(files []*zip.File) string {
	top := ""
	for _, f := range files {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		i := strings.IndexByte(name, '/')
		if i <= 0 {
			return "" // a file at the root
		}
		d := name[:i+1]
		if top == "" {
			top = d
		} else if d != top {
			return ""
		}
	}
	return top
}

// SwapDir replaces live with staged. The old live folder is kept as prev
// (replacing any older prev) so a bad update can be rolled back by hand.
// If the second rename fails, the original is restored.
func SwapDir(staged, live, prev string) error {
	if _, err := os.Stat(live); err == nil {
		if err := os.RemoveAll(prev); err != nil {
			return fmt.Errorf("remove old backup: %w", err)
		}
		if err := os.Rename(live, prev); err != nil {
			return fmt.Errorf("game folder is in use (close the game and anything open in that folder): %w", err)
		}
		if err := os.Rename(staged, live); err != nil {
			if rerr := os.Rename(prev, live); rerr != nil {
				return fmt.Errorf("swap failed (%v) and restore failed (%v); previous copy is at %s", err, rerr, prev)
			}
			return fmt.Errorf("swap failed, previous version restored: %w", err)
		}
		return nil
	}
	return os.Rename(staged, live)
}

// ReplaceFile moves newFile to live, keeping the old one as prev.
func ReplaceFile(newFile, live, prev string) error {
	if _, err := os.Stat(live); err == nil {
		os.Remove(prev)
		if err := os.MkdirAll(filepath.Dir(prev), 0o755); err != nil {
			return err
		}
		if err := os.Rename(live, prev); err != nil {
			return fmt.Errorf("game file is in use (close the game): %w", err)
		}
	}
	if err := os.Rename(newFile, live); err != nil {
		os.Rename(prev, live)
		return err
	}
	return nil
}
