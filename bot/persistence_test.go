package bot

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func mkPersistedSpot(t *testing.T, freq string, min int, qrt bool) DisplaySpot {
	t.Helper()
	return DisplaySpot{
		ID:        "POTA-1",
		Source:    "POTA",
		RawTime:   time.Date(2026, 4, 29, 8, min, 0, 0, time.UTC),
		Location:  "US-0189 (Don Edwards SF Bay NWR US-CA)",
		Frequency: freq,
		Mode:      "CW",
		Comment:   "QRV",
		QRT:       qrt,
	}
}

func openTempStore(t *testing.T) (*SpotStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spots.db")
	store, err := OpenSpotStore(path)
	if err != nil {
		t.Fatalf("OpenSpotStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store, path
}

func TestOpenSpotStore_CreatesFreshDB(t *testing.T) {
	store, path := openTempStore(t)

	got, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on fresh DB: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("fresh DB should be empty, got %d entries", len(got))
	}

	cols, err := store.tableColumns()
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	for _, c := range spotsSchema {
		if !cols[c.Name] {
			t.Errorf("fresh DB missing column %q", c.Name)
		}
	}

	if path == "" {
		t.Fatal("expected store path to be set")
	}
}

func TestSpotStore_RoundTrip(t *testing.T) {
	store, _ := openTempStore(t)

	spots := []DisplaySpot{
		mkPersistedSpot(t, "14044.0", 30, false),
		mkPersistedSpot(t, "14044.1", 35, true),
	}
	if err := store.ReplaceCallsign("W6JY", spots); err != nil {
		t.Fatalf("ReplaceCallsign: %v", err)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	got, ok := loaded["W6JY"]
	if !ok {
		t.Fatalf("expected W6JY in loaded map, got keys %v", loaded)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 spots, got %d", len(got))
	}
	// LoadAll runs through dedupAndSortSpots → newest-first.
	if got[0].Frequency != "14044.1" || !got[0].QRT {
		t.Errorf("entry 0: got %+v, want 14044.1 QRT", got[0])
	}
	if got[1].Frequency != "14044.0" || got[1].QRT {
		t.Errorf("entry 1: got %+v, want 14044.0 not-QRT", got[1])
	}
}

func TestSpotStore_ReplaceCallsignReplaces(t *testing.T) {
	store, _ := openTempStore(t)

	first := []DisplaySpot{mkPersistedSpot(t, "14044.0", 30, false)}
	if err := store.ReplaceCallsign("W6JY", first); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	second := []DisplaySpot{
		mkPersistedSpot(t, "7030.0", 40, false),
		mkPersistedSpot(t, "7030.0", 45, true),
	}
	if err := store.ReplaceCallsign("W6JY", second); err != nil {
		t.Fatalf("second replace: %v", err)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	got := loaded["W6JY"]
	if len(got) != 2 {
		t.Fatalf("expected 2 spots after replace, got %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Frequency == "14044.0" {
			t.Errorf("old spot %q should have been replaced", s.Frequency)
		}
	}
}

func TestSpotStore_PersistsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	store, err := OpenSpotStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := store.ReplaceCallsign("W6JY", []DisplaySpot{mkPersistedSpot(t, "14044.0", 30, false)}); err != nil {
		t.Fatalf("ReplaceCallsign: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := OpenSpotStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	loaded, err := reopened.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if got := loaded["W6JY"]; len(got) != 1 || got[0].Frequency != "14044.0" {
		t.Errorf("expected reopened DB to keep one 14044.0 spot, got %+v", got)
	}
}

func TestSpotStore_MigrationAddsMissingColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate.db")

	// Hand-roll a legacy table that's missing the qrt and comment columns.
	raw, err := sql.Open(sqliteDriver, path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE spots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		callsign TEXT NOT NULL DEFAULT '',
		spot_id TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		raw_time INTEGER NOT NULL DEFAULT 0,
		location TEXT NOT NULL DEFAULT '',
		frequency TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO spots (callsign, spot_id, source, raw_time, location, frequency, mode)
		VALUES ('W6JY', 'POTA-1', 'POTA', 0, 'US-0189', '14044.0', 'CW')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	store, err := OpenSpotStore(path)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	defer store.Close()

	cols, err := store.tableColumns()
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if !cols["qrt"] || !cols["comment"] {
		t.Errorf("migration should have added qrt and comment columns, got %v", cols)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll after migration: %v", err)
	}
	got := loaded["W6JY"]
	if len(got) != 1 {
		t.Fatalf("expected legacy row preserved, got %d entries", len(got))
	}
	if got[0].QRT || got[0].Comment != "" {
		t.Errorf("new columns should default to zero values, got QRT=%v comment=%q", got[0].QRT, got[0].Comment)
	}
}

func TestSpotStore_MigrationDropsExtraColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drop.db")

	raw, err := sql.Open(sqliteDriver, path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	// Schema with all current columns plus an obsolete 'old_field' that
	// migrate() should remove.
	if _, err := raw.Exec(`CREATE TABLE spots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		callsign TEXT NOT NULL DEFAULT '',
		spot_id TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		raw_time INTEGER NOT NULL DEFAULT 0,
		location TEXT NOT NULL DEFAULT '',
		frequency TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT '',
		qrt INTEGER NOT NULL DEFAULT 0,
		old_field TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create table with extra column: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	store, err := OpenSpotStore(path)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	defer store.Close()

	cols, err := store.tableColumns()
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if cols["old_field"] {
		t.Errorf("migration should have dropped 'old_field', columns: %v", cols)
	}
}

func TestUpdateCache_WritesToStore(t *testing.T) {
	store, _ := openTempStore(t)
	cacheMu.Lock()
	prevCache := spotCache
	prevStore := spotPersistence
	spotCache = make(map[string][]DisplaySpot)
	spotPersistence = store
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		spotCache = prevCache
		spotPersistence = prevStore
		cacheMu.Unlock()
	})

	updateCache("W6JY", mkPersistedSpot(t, "14044.0", 30, false))
	updateCache("W6JY", mkPersistedSpot(t, "14044.1", 35, true))

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	got := loaded["W6JY"]
	if len(got) != 2 {
		t.Fatalf("expected store to mirror cache (2 spots), got %d", len(got))
	}
}

func TestUpdateCache_RolloverTrimsStore(t *testing.T) {
	store, _ := openTempStore(t)
	cacheMu.Lock()
	prevCache := spotCache
	prevStore := spotPersistence
	spotCache = make(map[string][]DisplaySpot)
	spotPersistence = store
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		spotCache = prevCache
		spotPersistence = prevStore
		cacheMu.Unlock()
	})

	for i := 0; i < maxCachedSpots+5; i++ {
		updateCache("W6JY", DisplaySpot{
			ID:        "POTA-X",
			Source:    "POTA",
			RawTime:   time.Date(2026, 4, 29, 8, i, 0, 0, time.UTC),
			Location:  "US-0189",
			Frequency: string(rune('A' + i)),
			Mode:      "CW",
		})
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	got := loaded["W6JY"]
	if len(got) != maxCachedSpots {
		t.Fatalf("expected store to roll over to %d spots, got %d", maxCachedSpots, len(got))
	}
}

func TestHydrateCacheFromStore_PopulatesMemory(t *testing.T) {
	store, _ := openTempStore(t)
	if err := store.ReplaceCallsign("W6JY", []DisplaySpot{mkPersistedSpot(t, "14044.0", 30, false)}); err != nil {
		t.Fatalf("ReplaceCallsign: %v", err)
	}

	cacheMu.Lock()
	prevCache := spotCache
	prevStore := spotPersistence
	spotCache = make(map[string][]DisplaySpot)
	spotPersistence = nil
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		spotCache = prevCache
		spotPersistence = prevStore
		cacheMu.Unlock()
	})

	if err := hydrateCacheFromStore(store); err != nil {
		t.Fatalf("hydrateCacheFromStore: %v", err)
	}

	got := getCachedSpots("W6JY")
	if len(got) != 1 || got[0].Frequency != "14044.0" {
		t.Errorf("hydrated cache should hold the persisted spot, got %+v", got)
	}
}
