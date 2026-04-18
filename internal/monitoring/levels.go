package monitoring

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"space-invaders-coop/backend-go/internal/game"
)

// HandleLevelsList responds to GET /api/levels with the list of available levels.
func HandleLevelsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	files, err := game.ListLevelFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"levels":  files,
		"current": game.CurrentLevelName(),
	})
}

// HandleLevelByName routes GET/PUT/POST/DELETE /api/levels/<name>.
func HandleLevelByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/levels/")
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "invalid level name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := game.ReadLevelFile(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodPut, http.MethodPost:
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := game.WriteLevelFile(name, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if name == game.CurrentLevelName() {
			if err := game.ReloadCurrentLevel(); err != nil {
				http.Error(w, fmt.Sprintf("saved, but reload failed: %v", err), http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		if err := game.DeleteLevelFile(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleLevelsReload re-reads the active level from disk.
func HandleLevelsReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := game.ReloadCurrentLevel(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
