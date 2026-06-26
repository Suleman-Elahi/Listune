package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ilfs/listune-server/handlers"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Google SSO
	mux.HandleFunc("GET /auth/google", handlers.GoogleLogin)
	mux.HandleFunc("GET /auth/google/callback", handlers.GoogleCallback)

	// Library: scan server folder, stream files, serve artwork
	mux.HandleFunc("POST /api/library/scan", handlers.ScanFolder)
	mux.HandleFunc("GET /api/stream", handlers.StreamFile)
	mux.HandleFunc("GET /api/artwork", handlers.GetArtwork)

	// MusicBrainz / AcoustID identification
	mux.HandleFunc("POST /api/identify", handlers.IdentifyTrack)

	// Write ID3 tags to file
	mux.HandleFunc("POST /api/write-tags", handlers.WriteTags)

	// Wrap with CORS
	handler := handlers.CORSMiddleware(mux)

	log.Printf("Listune server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}
