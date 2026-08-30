package server

// Cross-device userdata sync: My List, playback progress and the
// Continue Watching list live in the browser's localStorage; these endpoints
// mirror them to a server-side JSON file so every device sees the same state.
// The client POSTs its local state and gets the merged result back; per-item
// timestamps decide winners for progress/continue-watching, My List is a
// union. Nothing here is per-account â€” one shared household state â€” matching
// the single shared password gate.

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
)

// progressEntry mirrors the client's per-title playback progress record.
type progressEntry struct {
	Season   int     `json:"season"`
	Episode  int     `json:"episode"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	// At is the client clock (ms since epoch) of the last save â€” the merge
	// winner for concurrent edits from two devices.
	At int64 `json:"at,omitempty"`
}

// userDataState is the full synced household state.
type userDataState struct {
	MyList   []json.RawMessage        `json:"mylist"`
	Progress map[string]*progressEntry `json:"progress"`
	CW       []json.RawMessage        `json:"cw"`
	// Removed maps mediaKey â†’ removal timestamp (client clock ms): a
	// tombstone that keeps one device's deletion from being resurrected by
	// another device's older copy. Entries newer than the tombstone
	// (re-adds) survive it.
	Removed map[string]int64 `json:"removed,omitempty"`
	// AVPrefs is the remembered audio/subtitle language choice
	// {"audio": "eng", "sub": "eng"|"off"}; PrefsAt is its client clock —
	// the whole blob is last-writer-wins.
	AVPrefs   map[string]string `json:"avprefs,omitempty"`
	AVPrefsAt int64             `json:"avprefs_at,omitempty"`
}

// userDataStore holds one state per account behind one mutex and mirrors it
// to disk.
type userDataStore struct {
	mu   sync.Mutex
	path string
	// states is keyed by the authenticated user's id.
	states map[string]*userDataState
	// legacy holds a pre-accounts single-household file, seeded into the
	// first account that registers.
	legacy *userDataState
}

func NewUserDataStore(path string) *userDataStore {
	s := &userDataStore{path: path, states: map[string]*userDataState{}}
	if path == "" || path == "-" {
		s.path = ""
	}
	if s.path == "" {
		return s
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s // first run
	}
	// v2: {"version":2,"users":{"<uid>":{...}}}. Legacy v1 files are the
	// old single-household state â€” kept aside and adopted by the first
	// account so an upgrade keeps its data.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		log.Printf("[UserData] discarding unreadable userdata file %s: %v", path, err)
		return s
	}
	if raw, ok := probe["users"]; ok {
		var users map[string]*userDataState
		if err := json.Unmarshal(raw, &users); err == nil {
			for uid, st := range users {
				if st != nil {
					if st.Progress == nil {
						st.Progress = map[string]*progressEntry{}
					}
					s.states[uid] = st
				}
			}
		}
		log.Printf("[UserData] restored userdata for %d account(s) from %s", len(s.states), path)
		return s
	}
	var single userDataState
	if err := json.Unmarshal(data, &single); err == nil &&
		(single.MyList != nil || single.Progress != nil || single.CW != nil) {
		if single.Progress == nil {
			single.Progress = map[string]*progressEntry{}
		}
		s.legacy = &single
		log.Printf("[UserData] found pre-accounts userdata; it will be adopted by the first registered account")
	}
	return s
}

// persistLocked writes all states atomically. Callers hold s.mu.
func (s *userDataStore) persistLocked() {
	if s.path == "" {
		return
	}
	payload := struct {
		Version int                           `json:"version"`
		Users   map[string]*userDataState     `json:"users"`
	}{Version: 2, Users: s.states}
	data, err := json.MarshalIndent(&payload, "", "  ")
	if err != nil {
		log.Printf("[UserData] marshal failed: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("[UserData] write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		log.Printf("[UserData] rename failed: %v", err)
	}
}

// stateFor returns the caller's state, creating it on first use. Callers
// are always authenticated (the handlers reject anonymous requests).
func (s *userDataStore) stateFor(userID string) *userDataState {
	if st, ok := s.states[userID]; ok {
		return st
	}
	st := &userDataState{Progress: map[string]*progressEntry{}}
	// Upgrade path: the first account adopts the pre-accounts household data.
	if userID != "" && s.legacy != nil {
		*st = *s.legacy
		if st.Progress == nil {
			st.Progress = map[string]*progressEntry{}
		}
		s.legacy = nil
		log.Printf("[UserData] adopted legacy userdata into account %s", userID)
	}
	s.states[userID] = st
	s.persistLocked()
	return st
}

// DeleteUserState drops a deleted account's synced data.
func (s *userDataStore) DeleteUserState(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.states[userID]; ok {
		delete(s.states, userID)
		s.persistLocked()
	}
}

// mediaRefKey extracts the client's mediaKey (`type-id`) from an opaque
// movie object so lists can be deduplicated across devices.
func mediaRefKey(raw json.RawMessage) string {
	var ref struct {
		ID   any    `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil || ref.ID == nil {
		return ""
	}
	t := ref.Type
	if t == "" {
		t = "movie"
	}
	switch id := ref.ID.(type) {
	case string:
		return t + "-" + id
	case float64:
		return t + "-" + strings.TrimSuffix(strings.TrimSuffix(fmtFloat(id), "0"), ".")
	default:
		return ""
	}
}

func fmtFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// merge folds the client's state into the store:
//   - progress: per media key, newer At wins;
//   - continue watching: per media key, newer At wins, newest-first, cap 20;
//   - my list: union (client order first, then server-only entries).
func (st *userDataState) merge(in userDataState) userDataState {
	merged := userDataState{
		Progress: map[string]*progressEntry{},
		MyList:   []json.RawMessage{},
		CW:       []json.RawMessage{},
	}

	// My List: union by media key, client order first.
	seenList := map[string]bool{}
	for _, raw := range in.MyList {
		k := mediaRefKey(raw)
		if k == "" || seenList[k] {
			continue
		}
		seenList[k] = true
		merged.MyList = append(merged.MyList, raw)
	}
	existingList := map[string]bool{}
	for _, raw := range merged.MyList {
		existingList[mediaRefKey(raw)] = true
	}
	for _, raw := range st.MyList {
		k := mediaRefKey(raw)
		if k == "" || existingList[k] {
			continue
		}
		existingList[k] = true
		merged.MyList = append(merged.MyList, raw)
	}

	// Progress: start from server, overwrite per key with newer client edits.
	for k, v := range st.Progress {
		if v != nil {
			merged.Progress[k] = v
		}
	}
	for k, v := range in.Progress {
		if v == nil {
			continue
		}
		if old, ok := merged.Progress[k]; ok {
			merged.Progress[k] = betterProgressEntry(old, v)
		} else {
			merged.Progress[k] = v
		}
	}

	// Continue watching: newest-wins per key, then order by At desc, cap 20.
	cwByKey := map[string]json.RawMessage{}
	atByKey := map[string]int64{}
	for k, v := range st.cwAt() {
		cwByKey[k], atByKey[k] = v.raw, v.at
	}
	for k, v := range in.cwAt() {
		if old, ok := atByKey[k]; !ok || v.at >= old {
			cwByKey[k], atByKey[k] = v.raw, v.at
		}
	}
	keys := make([]string, 0, len(cwByKey))
	for k := range cwByKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return atByKey[keys[i]] > atByKey[keys[j]] })
	for i, k := range keys {
		if i >= 20 {
			break
		}
		merged.CW = append(merged.CW, cwByKey[k])
	}

	// Tombstones: union keeping the newest removal per keyâ€¦
	merged.Removed = map[string]int64{}
	for k, at := range st.Removed {
		merged.Removed[k] = at
	}
	for k, at := range in.Removed {
		if at > merged.Removed[k] {
			merged.Removed[k] = at
		}
	}
	// â€¦then drop list/CW entries the tombstones outrank. Entries carry
	// their own timestamps (listAt / at); anything older than the
	// tombstone stays deleted everywhere. My List items are filtered
	// against their listAt field by re-reading the raw objects.
	merged.MyList = filterTombstoned(merged.MyList, "listAt", merged.Removed)
	merged.CW = filterTombstoned(merged.CW, "at", merged.Removed)

	// A/V preferences: single blob, newest client clock wins.
	if in.AVPrefsAt > st.AVPrefsAt {
		merged.AVPrefs = in.AVPrefs
		merged.AVPrefsAt = in.AVPrefsAt
	} else {
		merged.AVPrefs = st.AVPrefs
		merged.AVPrefsAt = st.AVPrefsAt
	}
	return merged
}

// betterProgressEntry picks the winner between the stored entry and an
// incoming client edit for the same media key. Normally the newer client
// clock wins — but a position-less stamp (no position and no duration, e.g.
// a client that only recorded "user opened S1E2") must not erase a real
// playback position for the same season/episode, whatever the clock says:
// one device merely opening an episode would otherwise reset the position
// another device actually watched to. Different season/episode means the
// user moved on, so the newer entry always wins there.
func betterProgressEntry(old, new *progressEntry) *progressEntry {
	oldStamp := old.Position == 0 && old.Duration == 0
	newStamp := new.Position == 0 && new.Duration == 0
	if oldStamp != newStamp && old.Season == new.Season && old.Episode == new.Episode {
		if oldStamp {
			return new
		}
		return old
	}
	if new.At >= old.At {
		return new
	}
	return old
}

// filterTombstoned removes entries whose media key has a removal timestamp
// newer than the entry's own timestamp.
func filterTombstoned(list []json.RawMessage, atField string, removed map[string]int64) []json.RawMessage {
	if len(removed) == 0 {
		return list
	}
	out := make([]json.RawMessage, 0, len(list))
	for _, raw := range list {
		k := mediaRefKey(raw)
		if k == "" {
			out = append(out, raw)
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(raw, &meta)
		at := int64(0)
		if v, ok := meta[atField]; ok {
			if f, ok := v.(float64); ok {
				at = int64(f)
			}
		}
		if tomb, ok := removed[k]; ok && tomb >= at {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// cwAt pairs continue-watching entries with their timestamps.
type cwEntry struct {
	raw json.RawMessage
	at  int64
}

func (st userDataState) cwAt() map[string]cwEntry {
	out := map[string]cwEntry{}
	for _, raw := range st.CW {
		k := mediaRefKey(raw)
		if k == "" {
			continue
		}
		var meta struct {
			At int64 `json:"at"`
		}
		_ = json.Unmarshal(raw, &meta)
		out[k] = cwEntry{raw: raw, at: meta.At}
	}
	return out
}

// userdataGetHandler returns the caller's full synced state. Anonymous
// callers get 401 — persistence is the one thing accounts provide; the
// client falls back to localStorage-only mode.
func (d *Deps) userdataGetHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Sign in to sync your data")
		return
	}
	d.UserData.mu.Lock()
	defer d.UserData.mu.Unlock()
	writeJSON(w, http.StatusOK, *d.UserData.stateFor(userID))
}

// userdataSyncHandler merges the caller's local state and returns the merged
// result for the client to write back into localStorage. Each account has
// its own state, so devices signed into the same account share everything
// and different accounts stay fully isolated. Anonymous callers get 401 —
// their client stays in localStorage-only mode.
func (d *Deps) userdataSyncHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Sign in to sync your data")
		return
	}
	var in userDataState
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid userdata payload")
		return
	}
	if d.UserData == nil {
		// Persistence disabled: echo the client state back with defaults so
		// the API contract holds even without a store.
		if in.Progress == nil {
			in.Progress = map[string]*progressEntry{}
		}
		if in.MyList == nil {
			in.MyList = []json.RawMessage{}
		}
		if in.CW == nil {
			in.CW = []json.RawMessage{}
		}
		writeJSON(w, http.StatusOK, in)
		return
	}
	d.UserData.mu.Lock()
	st := d.UserData.stateFor(userID)
	merged := st.merge(in)
	*st = merged
	d.UserData.persistLocked()
	d.UserData.mu.Unlock()
	writeJSON(w, http.StatusOK, merged)
}

// userdataClearHandler wipes the caller's entire synced state (My List,
// playback progress, continue watching, preferences). The account itself
// stays; the next sync re-uploads whatever the current device still holds.
func (d *Deps) userdataClearHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Sign in first")
		return
	}
	if d.UserData != nil {
		d.UserData.mu.Lock()
		d.UserData.states[userID] = &userDataState{
			MyList:   []json.RawMessage{},
			Progress: map[string]*progressEntry{},
			CW:       []json.RawMessage{},
		}
		d.UserData.persistLocked()
		d.UserData.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
