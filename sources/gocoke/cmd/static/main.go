package main

import (
	"flag"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

func main() {
	// Ensure correct MIME type for WebAssembly files
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	root := flag.String("root", ".", "directory to serve")
	flag.Parse()

	fs := http.Dir(*root)
	fileServer := http.FileServer(fs)

	// Prefer precompressed Brotli assets when available and requested by the client.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only attempt for safe GET/HEAD requests
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if ae := r.Header.Get("Accept-Encoding"); strings.Contains(ae, "br") {
				// Try to serve a sibling .br file (e.g. main.wasm.br)
				// Compute original content-type from the uncompressed extension
				origExt := filepath.Ext(r.URL.Path)
				if ct := mime.TypeByExtension(origExt); ct != "" {
					w.Header().Set("Content-Type", ct)
				}
				// Open precompressed variant
				if f, err := fs.Open(r.URL.Path + ".br"); err == nil {
					defer f.Close()
					if fi, err := f.Stat(); err == nil && !fi.IsDir() {
						w.Header().Set("Content-Encoding", "br")
						w.Header().Set("Vary", "Accept-Encoding")
						// Serve the precompressed content with range support
						http.ServeContent(w, r, r.URL.Path, fi.ModTime(), f)
						return
					}
				}
			}
		}
		// Fallback to the regular file server
		fileServer.ServeHTTP(w, r)
	})

	log.Printf("Serving %s at http://%s/", *root, *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}
