package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/rs/zerolog/log"
)

// Storage manages two DuckDB connections (writer + reader), the batch buffer
// system, and the in-memory LiveCache for the latest RaceState snapshot.
type Storage struct {
	writer  *sql.DB    // single writer connection — all INSERTs go here
	reader  *sql.DB    // separate reader for concurrent API queries
	buffers *BufferSet
	cache   *LiveCache
	dbPath  string
}

// NewStorage opens a writer and a reader connection to the DuckDB database at
// dbPath, initialises the schema (all 10 tables), and creates the batch buffer
// set with the requested batchSize.
func NewStorage(dbPath string, batchSize int) (*Storage, error) {
	writer, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	// Enforce a single writer connection to avoid DuckDB write contention.
	writer.SetMaxOpenConns(1)

	// Create all tables via the writer BEFORE opening the read-only connection.
	// DuckDB's read-only connection takes a catalog snapshot at open time, so
	// tables must exist first or the reader won't see them.
	if err := InitSchema(writer); err != nil {
		writer.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	reader, err := sql.Open("duckdb", dbPath+"?access_mode=READ_ONLY")
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}

	s := &Storage{
		writer:  writer,
		reader:  reader,
		buffers: NewBufferSet(writer, batchSize),
		cache:   NewLiveCache(),
		dbPath:  dbPath,
	}

	log.Info().Str("path", dbPath).Int("batch", batchSize).Msg("storage initialised")
	return s, nil
}

// Writer returns the single-writer DuckDB connection.
func (s *Storage) Writer() *sql.DB { return s.writer }

// Reader returns the read-only DuckDB connection for concurrent API queries.
func (s *Storage) Reader() *sql.DB { return s.reader }

// Buffers returns the set of table batch buffers.
func (s *Storage) Buffers() *BufferSet { return s.buffers }

// Cache returns the in-memory LiveCache holding the latest RaceState.
func (s *Storage) Cache() *LiveCache { return s.cache }

// Query executes an arbitrary read-only SQL statement via the reader
// connection and returns the result rows as a slice of column-name-keyed maps.
// This powers the /api/query endpoint.
func (s *Storage) Query(ctx context.Context, query string) ([]map[string]interface{}, error) {
	// Flush all pending writes so the reader sees the latest data.
	if err := s.buffers.FlushAll(ctx); err != nil {
		log.Warn().Err(err).Msg("flush before query failed, results may be stale")
	}

	rows, err := s.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		// Create a slice of interface{} pointers for Scan.
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}

// FlushAll forces all batch buffers to write their pending rows to DuckDB.
func (s *Storage) FlushAll(ctx context.Context) error {
	return s.buffers.FlushAll(ctx)
}

// Close flushes all pending data and closes both database connections.
func (s *Storage) Close() error {
	// Best-effort flush before closing.
	if err := s.buffers.FlushAll(context.Background()); err != nil {
		log.Error().Err(err).Msg("flush on close failed")
	}

	var firstErr error
	if err := s.writer.Close(); err != nil {
		firstErr = fmt.Errorf("close writer: %w", err)
	}
	if err := s.reader.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("close reader: %w", err)
	}
	return firstErr
}
