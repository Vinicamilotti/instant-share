package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"instant-share/handler"
)

//go:embed web/dist/**
var staticFiles embed.FS

func main() {
	addr := os.Getenv("INSTANT_SHARE_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}

	staticFS, err := fs.Sub(staticFiles, "web/dist")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/host-session", handler.HostSession)
	mux.HandleFunc("GET /ws/{session_id}", handler.WebSocket)

	fileServer := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || staticFSFileExists(staticFS, path[1:]) {
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	log.Printf("instant-share listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func staticFSFileExists(fsys fs.FS, path string) bool {
	f, err := fsys.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}