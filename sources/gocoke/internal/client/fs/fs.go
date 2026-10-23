package fs

import (
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	webfs "gocoke/web"
)

// normalizePath converts Windows backslashes to URL-friendly forward slashes
// so that paths work both on desktop and in browsers (WASM).
func normalizePath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// Open opens a file path for reading in a way that works on desktop and WASM.
// On desktop it reads from the filesystem; on web it fetches via HTTP relative to the page.
func Open(path string) (io.ReadCloser, error) {
	p := normalizePath(path)
	f, err := webfs.Files.Open(p)
	if err != nil {
		// Retry with web/ prefix since we embedded under web/
		wp := filepath.ToSlash(filepath.Join("web", p))
		f, err = webfs.Files.Open(wp)
		if err != nil {
			return nil, err
		}
	}
	if rc, ok := f.(io.ReadCloser); ok {
		return rc, nil
	}
	return io.NopCloser(f.(fs.File)), nil
}

// ReadFile reads the entire file into memory.
func ReadFile(path string) ([]byte, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
