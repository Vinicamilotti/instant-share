package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"instant-share/handler"
)

//go:embed web/dist/**
var staticFiles embed.FS

func main() {
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

	log.Println("instant-share listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func staticFSFileExists(fsys fs.FS, path string) bool {
	f, err := fsys.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}