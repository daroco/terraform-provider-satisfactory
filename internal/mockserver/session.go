package mockserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Session control and inventory grants. The mock has no save files and no
// process to exit, so it mirrors the contract's shapes and error cases:
// the save name default, one save at a time, the player/item/count checks.
// What it cannot mirror is left to the mod (see the mod's
// docs/implementation-notes.md, "Session control").

// mockSessionName stands in for the game's session name.
const mockSessionName = "mock"

// SavedNames reports every save name the mock was asked to write, in order.
// Tests use it to check what a caller asked for.
func (s *Server) SavedNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.saved...)
}

// QuitRequested reports whether POST /session/quit was called.
func (s *Server) QuitRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quit
}

func (s *Server) saveSession(w http.ResponseWriter, r *http.Request) {
	var req api.SessionSave
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := s.recordSave(req.Name)
	writeJSON(w, http.StatusOK, api.SessionSaveResult{Saved: name})
}

func (s *Server) quitSession(w http.ResponseWriter, r *http.Request) {
	var req api.SessionQuit
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := ""
	if req.Save == nil || *req.Save {
		name = s.recordSave(req.Name)
	}
	s.quit = true
	writeJSON(w, http.StatusAccepted, api.SessionSaveResult{Saved: name})
}

// recordSave applies the default name and remembers the save. Caller holds s.mu.
func (s *Server) recordSave(name string) string {
	if name == "" {
		name = mockSessionName + "_terraform"
	}
	s.saved = append(s.saved, name)
	return name
}

func (s *Server) grantInventory(w http.ResponseWriter, r *http.Request) {
	var req api.InventoryGrant
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Count == 0 {
		req.Count = 1
	}
	if req.Count < 1 || req.Count > 100000 {
		writeErr(w, http.StatusUnprocessableEntity, "count must be between 1 and 100000")
		return
	}
	if !strings.HasPrefix(req.Item, "Desc_") || !strings.HasSuffix(req.Item, "_C") {
		writeErr(w, http.StatusUnprocessableEntity, "item must be an item descriptor class name like Desc_Coal_C")
		return
	}
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, p := range s.players {
		if p.Name == name {
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "no player named "+name)
		return
	}
	writeJSON(w, http.StatusOK, api.InventoryGrantResult{Player: name, Item: req.Item, Requested: req.Count, Added: req.Count})
}
