package bot

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

// columnDef describes a single column in the spots table. The migration logic
// uses Name to detect missing/extra columns and Type as the right-hand side of
// CREATE TABLE / ALTER TABLE ADD COLUMN. PrimaryKey columns are emitted only
// during initial CREATE — SQLite does not allow adding a primary-key column
// via ALTER TABLE, and won't drop one either.
type columnDef struct {
	Name       string
	Type       string
	PrimaryKey bool
}

// spotsSchema is the source of truth for the spots table. To extend the
// schema, add or remove entries here — the migrate() pass on next startup
// will reconcile the on-disk table (CREATE TABLE on a fresh DB; ALTER TABLE
// ADD/DROP COLUMN on an existing one). New non-PK columns must be nullable
// or carry a DEFAULT so the ALTER works on a populated table.
var spotsSchema = []columnDef{
	{Name: "id", Type: "INTEGER", PrimaryKey: true},
	{Name: "callsign", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "spot_id", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "source", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "raw_time", Type: "INTEGER NOT NULL DEFAULT 0"},
	{Name: "location", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "frequency", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "mode", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "comment", Type: "TEXT NOT NULL DEFAULT ''"},
	{Name: "qrt", Type: "INTEGER NOT NULL DEFAULT 0"},
}

// SpotStore persists the per-callsign spot cache to a SQLite file. The file
// is created on first open and tuned for SD-card-backed deployments
// (Raspberry Pi): WAL journaling avoids re-writing the same pages, NORMAL
// synchronous skips fsync on every commit, and the temp store stays in RAM.
type SpotStore struct {
	mu sync.Mutex
	db *sql.DB
}

// OpenSpotStore opens (or creates) the DB at path, applies SD-card-friendly
// pragmas, and reconciles the schema against spotsSchema.
func OpenSpotStore(path string) (*SpotStore, error) {
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}
	// One connection serializes writes and avoids WAL contention on slow IO.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA wal_autocheckpoint = 1000",
		"PRAGMA cache_size = -2000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %q: %w", p, err)
		}
	}

	s := &SpotStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close flushes pending writes and closes the underlying DB handle.
func (s *SpotStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// migrate creates the spots table on a fresh DB, otherwise reconciles its
// columns against spotsSchema by adding missing ones and dropping extras.
// Primary-key columns are never altered after initial creation.
func (s *SpotStore) migrate() error {
	var found string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='spots'`,
	).Scan(&found)
	switch {
	case err == sql.ErrNoRows:
		return s.createFresh()
	case err != nil:
		return fmt.Errorf("inspect sqlite_master: %w", err)
	}

	existing, err := s.tableColumns()
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(spotsSchema))
	for _, c := range spotsSchema {
		wanted[c.Name] = true
	}

	for _, c := range spotsSchema {
		if c.PrimaryKey || existing[c.Name] {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE spots ADD COLUMN %s %s", c.Name, c.Type)
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("add column %s: %w", c.Name, err)
		}
	}
	for name := range existing {
		if wanted[name] {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE spots DROP COLUMN %s", name)
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("drop column %s: %w", name, err)
		}
	}
	return nil
}

func (s *SpotStore) createFresh() error {
	parts := make([]string, 0, len(spotsSchema))
	for _, c := range spotsSchema {
		switch {
		case c.PrimaryKey:
			parts = append(parts, fmt.Sprintf("%s %s PRIMARY KEY AUTOINCREMENT", c.Name, c.Type))
		default:
			parts = append(parts, fmt.Sprintf("%s %s", c.Name, c.Type))
		}
	}
	stmt := fmt.Sprintf("CREATE TABLE spots (%s)", strings.Join(parts, ", "))
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("create spots table: %w", err)
	}
	if _, err := s.db.Exec(
		"CREATE INDEX idx_spots_callsign_time ON spots(callsign, raw_time DESC)",
	); err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}

func (s *SpotStore) tableColumns() (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(spots)`)
	if err != nil {
		return nil, fmt.Errorf("table_info: %w", err)
	}
	defer rows.Close()
	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info: %w", err)
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// LoadAll returns every persisted spot grouped by callsign, sorted/deduped
// the same way as the in-memory cache so the two stay in lockstep after a
// restart. Spots stored with the legacy schema (missing fields) round-trip
// through the column DEFAULTs.
func (s *SpotStore) LoadAll() (map[string][]DisplaySpot, error) {
	rows, err := s.db.Query(
		`SELECT callsign, spot_id, source, raw_time, location, frequency, mode, comment, qrt FROM spots`,
	)
	if err != nil {
		return nil, fmt.Errorf("select spots: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]DisplaySpot)
	for rows.Next() {
		var (
			callsign, spotID, source, location, frequency, mode, comment string
			rawTime                                                      int64
			qrt                                                          int
		)
		if err := rows.Scan(&callsign, &spotID, &source, &rawTime, &location, &frequency, &mode, &comment, &qrt); err != nil {
			return nil, fmt.Errorf("scan spot: %w", err)
		}
		out[callsign] = append(out[callsign], DisplaySpot{
			ID:        spotID,
			Source:    source,
			RawTime:   time.Unix(rawTime, 0).UTC(),
			Location:  location,
			Frequency: frequency,
			Mode:      mode,
			Comment:   comment,
			QRT:       qrt != 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for cs, spots := range out {
		out[cs] = dedupAndSortSpots(spots, maxCachedSpots)
	}
	return out, nil
}

// ReplaceCallsign mirrors the in-memory cache for a callsign: it deletes
// every persisted row for that callsign and writes the supplied slice in a
// single transaction. The snapshot approach keeps the on-disk state aligned
// with the in-memory dedup/rollover without tracking per-spot identity.
func (s *SpotStore) ReplaceCallsign(callsign string, spots []DisplaySpot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM spots WHERE callsign = ?`, callsign); err != nil {
		return fmt.Errorf("delete spots for %s: %w", callsign, err)
	}
	if len(spots) > 0 {
		stmt, err := tx.Prepare(
			`INSERT INTO spots (callsign, spot_id, source, raw_time, location, frequency, mode, comment, qrt)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		)
		if err != nil {
			return fmt.Errorf("prepare insert: %w", err)
		}
		defer stmt.Close()
		for _, sp := range spots {
			qrt := 0
			if sp.QRT {
				qrt = 1
			}
			if _, err := stmt.Exec(callsign, sp.ID, sp.Source, sp.RawTime.Unix(), sp.Location, sp.Frequency, sp.Mode, sp.Comment, qrt); err != nil {
				return fmt.Errorf("insert spot for %s: %w", callsign, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
