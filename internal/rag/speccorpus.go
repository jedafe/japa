// Package rag — speccorpus.go implements the Engineering Spec Corpus
// (§121, ADR-131): indexes the vendored .jeos/ spec tree into
// content_embeddings as private spec_doc chunks and exposes the staff-only
// privileged retrieval path backing the agent's spec.search tool.
//
// Indexing is idempotent: rag_spec_sources.file_sha256 skips unchanged files
// and a sweep pass purges registry rows + chunks for paths no longer on
// disk. spec_doc rows are always visibility='private' with NULL
// space_id/author_id — they are reachable only through RetrievePrivileged,
// which uses the dedicated SearchSpecEmbeddings query that hard-requires
// entity_type='spec_doc' AND visibility='private', so no caller bug can
// widen spec retrieval into user content.
//
// Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md
//
//	japaconnect-engineering/adr/ADR-131-engineering-spec-corpus.md
//	japaconnect-engineering/sections/69-rag-knowledge-layer.md (scoped retrieval)
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// ErrSpecCorpusDisabled is returned when the rag.spec_corpus feature flag is
// off — the indexer task and the spec.search tool both gate on it.
var ErrSpecCorpusDisabled = errors.New("rag: spec corpus disabled (feature flag rag.spec_corpus)")

// specCorpusFileExts is the set of indexed extensions under the corpus root.
var specCorpusFileExts = map[string]bool{
	".md":   true,
	".yaml": true,
	".yml":  true,
}

// specCorpusMaxFileBytes skips oversized files (generated dumps, vendored
// blobs) — 200 KB per §121.
const specCorpusMaxFileBytes = 200 * 1024

// SpecEntityID returns the deterministic entity UUID for a corpus-relative
// path: UUIDv5(URL namespace, "jeos:"+path). Re-indexing a file maps to the
// same entity; a renamed file is a new entity and the stale one is swept.
func SpecEntityID(relPath string) pgtype.UUID {
	u := uuid.NewSHA1(uuid.NameSpaceURL, []byte("jeos:"+relPath))
	return pgtype.UUID{Bytes: u, Valid: true}
}

// specCorpusEnabled resolves the rag.spec_corpus feature flag; the legacy
// system_settings key is consulted only as a fallback when no checker is
// wired (same pattern as ragEnabled — deprecated fallback, not a minted
// reachability toggle).
func (s *Service) specCorpusEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "rag.spec_corpus")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "rag.spec_corpus", false)
}

// SpecReindexStats summarizes a ReindexSpecCorpus run for logs and the admin
// route response.
type SpecReindexStats struct {
	FilesScanned       int `json:"files_scanned"`
	FilesIndexed       int `json:"files_indexed"`
	FilesSkipped       int `json:"files_skipped_unchanged"`
	FilesSkippedTooBig int `json:"files_skipped_too_big"`
	ChunksWritten      int `json:"chunks_written"`
	SourcesSwept       int `json:"sources_swept"`
}

// ReindexSpecCorpus walks dir (the corpus root — JC_SPEC_CORPUS_DIR or
// ".jeos"), indexes every *.md/*.yaml file as spec_doc chunks, and sweeps
// registry rows for paths that no longer exist. Content-hash skip makes
// re-runs cheap: unchanged files never re-embed.
func (s *Service) ReindexSpecCorpus(ctx context.Context, dir string) (*SpecReindexStats, error) {
	if !s.specCorpusEnabled(ctx) {
		return nil, ErrSpecCorpusDisabled
	}

	if s.embedder == nil {
		return nil, errors.New("rag: spec corpus: no embedder configured")
	}

	if dir == "" {
		dir = ".jeos"
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("rag: spec corpus: stat %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("rag: spec corpus: %s is not a directory", dir)
	}

	maxTokens := s.settings.GetIntOr(ctx, "rag.chunk_max_tokens", 400)
	if maxTokens < 1 {
		maxTokens = 1
	} else if maxTokens > 8000 {
		maxTokens = 8000
	}

	model := s.settings.GetStringOr(ctx, "rag.embedding_model", "text-embedding-3-small")

	stats := &SpecReindexStats{}
	onDisk := make(map[string]bool)

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// Never descend into VCS internals.
			if d.Name() == ".git" {
				return fs.SkipDir
			}

			return nil
		}

		if !specCorpusFileExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Canonical store form: forward slashes, no leading "./".
		rel = filepath.ToSlash(rel)
		stats.FilesScanned++
		onDisk[rel] = true

		fi, err := d.Info()
		if err != nil {
			return err
		}

		if fi.Size() > specCorpusMaxFileBytes {
			stats.FilesSkippedTooBig++
			slog.Warn("rag: spec corpus: skipping oversized file", "path", rel, "bytes", fi.Size())
			return nil
		}

		data, err := os.ReadFile(path) // #nosec G304 -- path is confined to the corpus dir by WalkDir
		if err != nil {
			return err
		}

		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])

		existing, err := s.store.SpecSourceByPath(ctx, rel)
		if err == nil && existing.FileSha256 == hash {
			stats.FilesSkipped++
			return nil
		}

		n, err := s.indexSpecFile(ctx, rel, specDocTitle(rel, string(data)), hash, string(data), model, maxTokens)
		if err != nil {
			return fmt.Errorf("rag: spec corpus: index %s: %w", rel, err)
		}

		stats.FilesIndexed++
		stats.ChunksWritten += n
		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	// Sweep: registry rows whose path is gone on disk lose their chunks too.
	registered, err := s.store.ListSpecSources(ctx)
	if err != nil {
		return stats, fmt.Errorf("rag: spec corpus: list sources for sweep: %w", err)
	}

	for _, src := range registered {
		if onDisk[src.Path] {
			continue
		}

		if err := s.store.DeleteEntityEmbeddings(ctx, EntityTypeSpecDoc, src.ID); err != nil {
			slog.Warn("rag: spec corpus: sweep purge chunks failed", "path", src.Path, "error", err)
		}

		if err := s.store.DeleteSpecSource(ctx, src.ID); err != nil {
			slog.Warn("rag: spec corpus: sweep delete source failed", "path", src.Path, "error", err)
			continue
		}

		stats.SourcesSwept++
	}

	slog.Info("rag: spec corpus: reindex complete",
		"scanned", stats.FilesScanned,
		"indexed", stats.FilesIndexed,
		"skipped_unchanged", stats.FilesSkipped,
		"skipped_too_big", stats.FilesSkippedTooBig,
		"chunks", stats.ChunksWritten,
		"swept", stats.SourcesSwept)

	return stats, nil
}

// QueueSpecReindex enqueues the rag:reindex_specs Asynq task (§11 catalog).
// The admin POST /admin/rag/reindex-specs route calls this. Returns
// ErrSpecCorpusDisabled when the rag.spec_corpus flag is off.
func (s *Service) QueueSpecReindex(ctx context.Context, dir string) error {
	if !s.specCorpusEnabled(ctx) {
		return ErrSpecCorpusDisabled
	}

	if s.enqueuer == nil {
		return errors.New("rag: spec corpus: no task enqueuer configured")
	}

	payload, err := json.Marshal(map[string]string{"dir": dir})
	if err != nil {
		return fmt.Errorf("rag: spec corpus: marshal payload: %w", err)
	}

	return s.enqueuer.Enqueue(worker.TypeRAGReindexSpecs, payload)
}

// indexSpecFile chunks, embeds, and upserts one spec file's chunks plus its
// registry row. Returns the number of chunks written.
func (s *Service) indexSpecFile(ctx context.Context, rel, title, hash, body, model string, maxTokens int) (int, error) {
	entityID := SpecEntityID(rel)

	sections := ChunkSpecDoc(body, maxTokens)
	if len(sections) == 0 {
		// Empty file — purge any chunks and drop the registry row.
		if err := s.store.DeleteEntityEmbeddings(ctx, EntityTypeSpecDoc, entityID); err != nil {
			return 0, err
		}

		return 0, s.store.DeleteSpecSourceByPath(ctx, rel)
	}

	ref := EntityRef{
		Type:       EntityTypeSpecDoc,
		ID:         entityID,
		Visibility: "private",
	}

	prefix := "[spec: " + rel + "]\n\n"

	for i, text := range sections {
		chunkText := prefix + text

		vec, err := s.embedder.GenerateEmbedding(ctx, chunkText, model)
		if err != nil {
			return 0, fmt.Errorf("embed chunk %d: %w", i, err)
		}

		if len(vec) == 0 {
			return 0, fmt.Errorf("empty embedding for chunk %d", i)
		}

		if err := s.store.UpsertChunk(ctx, ref, Chunk{
			Index:  i,
			Text:   chunkText,
			Vector: vec,
		}, "en"); err != nil {
			return 0, fmt.Errorf("upsert chunk %d: %w", i, err)
		}
	}

	// Delete stale tail chunks from a previous, longer index.
	//nolint:gosec // G115: chunk count is bounded by file size
	if err := s.store.DeleteStaleTail(ctx, EntityTypeSpecDoc, entityID, int32(len(sections))); err != nil { // #nosec G115 -- bounded
		slog.Warn("rag: spec corpus: delete stale tail failed", "path", rel, "error", err)
	}

	if err := s.store.UpsertSpecSource(ctx, entityID, rel, title, hash, int32(len(sections))); err != nil { // #nosec G115 -- bounded
		return 0, fmt.Errorf("upsert spec source: %w", err)
	}

	return len(sections), nil
}

// specDocTitle extracts the display title: the first markdown H1, else the
// base filename.
func specDocTitle(rel, body string) string {
	for line := range strings.Lines(body) {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			return strings.TrimSpace(t)
		}
	}

	return filepath.Base(rel)
}

// RetrievePrivileged performs privileged retrieval over the engineering spec
// corpus — the §69 scoped-retrieval CanSeePrivate branch's first production
// caller (§121). It deliberately uses the dedicated SearchSpecEmbeddings
// query, which only ever touches entity_type='spec_doc' rows with
// visibility='private': the privilege boundary is enforced by the query
// shape itself, not by trusting callers to pass the right scope.
//
// pathPrefix optionally narrows results to a subtree ('sections/', 'adr/').
// Results are cached under the same jc:rag:query:{hash} family with the
// privileged bit folded into the scope hash — privileged results can never
// cross into unprivileged cache entries.
func (s *Service) RetrievePrivileged(ctx context.Context, query, language, pathPrefix string, topK int32) ([]RetrievedChunk, error) {
	if !s.specCorpusEnabled(ctx) {
		return nil, ErrSpecCorpusDisabled
	}

	if !s.ragEnabled(ctx) {
		return nil, nil
	}

	if s.embedder == nil {
		slog.Warn("rag: retrieve privileged: no embedder configured")
		return nil, nil
	}

	if query == "" {
		return nil, nil
	}

	model := s.settings.GetStringOr(ctx, "rag.embedding_model", "text-embedding-3-small")

	// Same clamp contract as Retrieve: [1, 50].
	if topK < 1 {
		topK = 1
	} else if topK > 50 {
		topK = 50
	}

	queryVec, err := s.embedder.GenerateEmbedding(ctx, query, model)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieve privileged: embed query: %w", err)
	}

	if len(queryVec) == 0 {
		return nil, errors.New("rag: retrieve privileged: empty query embedding")
	}

	// Privileged scope: CanSeePrivate, no viewer, no memberships.
	scope := RetrievalScope{CanSeePrivate: true}
	cacheKey := ragQueryCacheKey(queryVec, language, topK, nil, scope)

	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []RetrievedChunk
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				return results, nil
			}
		}
	}

	results, err := s.store.SearchSpecs(ctx, queryVec, language, topK, pathPrefix)
	if err != nil {
		return nil, err
	}

	if s.cache != nil && len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			if err := s.cache.Set(ctx, cacheKey, data, ragQueryCacheTTL).Err(); err != nil {
				slog.Warn("rag: retrieve privileged: cache set failed", "error", err)
			}
		}
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// Store methods — rag_spec_sources registry + privileged spec search.
// ---------------------------------------------------------------------------

// SpecSourceByPath returns the registry row for a corpus path; pgx.ErrNoRows
// propagates to callers that distinguish found/missing (the indexer treats
// any error as missing).
func (s *Store) SpecSourceByPath(ctx context.Context, path string) (gen.RagSpecSource, error) {
	return s.queries.GetSpecSourceByPath(ctx, path)
}

// UpsertSpecSource inserts or refreshes a registry row keyed by path.
func (s *Store) UpsertSpecSource(ctx context.Context, id pgtype.UUID, path, title, fileSHA256 string, chunkCount int32) error {
	_, err := s.writeQueries.UpsertSpecSource(ctx, gen.UpsertSpecSourceParams{
		ID:         id,
		Path:       path,
		Title:      title,
		FileSha256: fileSHA256,
		ChunkCount: chunkCount,
	})
	return err
}

// ListSpecSources enumerates registered (id, path) pairs for the reindex
// sweep.
func (s *Store) ListSpecSources(ctx context.Context) ([]gen.ListSpecSourcesRow, error) {
	return s.queries.ListSpecSources(ctx)
}

// DeleteSpecSource removes a registry row by entity ID (sweep path).
func (s *Store) DeleteSpecSource(ctx context.Context, id pgtype.UUID) error {
	return s.writeQueries.DeleteSpecSource(ctx, id)
}

// DeleteSpecSourceByPath removes a registry row by path (empty-file purge).
func (s *Store) DeleteSpecSourceByPath(ctx context.Context, path string) error {
	src, err := s.queries.GetSpecSourceByPath(ctx, path)
	if err != nil {
		return nil // not registered — nothing to do
	}

	return s.writeQueries.DeleteSpecSource(ctx, src.ID)
}

// SearchSpecs runs the privileged spec-corpus query — entity_type='spec_doc'
// and visibility='private' are hard-required by the SQL itself.
func (s *Store) SearchSpecs(ctx context.Context, queryVector []float32, language string, topK int32, pathPrefix string) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)

	var prefix *string
	if pathPrefix != "" {
		prefix = &pathPrefix
	}

	rows, err := s.queries.SearchSpecEmbeddings(ctx, gen.SearchSpecEmbeddingsParams{
		QueryEmbedding: &vec,
		Language:       language,
		TopK:           topK,
		PathPrefix:     prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: search spec embeddings: %w", err)
	}

	results := make([]RetrievedChunk, 0, len(rows))
	for _, row := range rows {
		results = append(results, RetrievedChunk{
			EntityID:   row.EntityID.String(),
			EntityType: row.EntityType,
			ChunkText:  row.ChunkText,
			Similarity: row.Similarity,
			Tier:       SourceTier(row.EntityType, ""),
			Title:      row.EntityTitle,
			Slug:       row.EntitySlug,
			URL:        "", // spec_doc has no frontend route (§121)
		})
	}

	return results, nil
}
