package fetcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// ocrCacheTTL is the TTL for the OCR result cache (30 days per
// valkey-keys-catalog Section INTEL).
const ocrCacheTTL = 30 * 24 * time.Hour

// ocrExecTimeout caps how long a single tesseract/pdftoppm invocation may
// run before being killed. A large/scanned PDF can otherwise hang the
// Asynq default queue indefinitely (BUG-013 fix, spec §970 graceful
// degradation). The worker's Asynq retry then routes the record to
// REVIEW_REQUIRED instead of wedging the queue.
const ocrExecTimeout = 120 * time.Second

// OCRFetcher runs tesseract (via pdftoppm for image conversion) to extract
// text from scanned PDFs. It is a fallback used by PDFFetcher when the
// embedded text layer is empty. Callers should check Available() before use
// and route to REVIEW_REQUIRED when tesseract is absent.
//
// When a Valkey client is provided via NewOCRFetcherWithCache, OCR results
// are cached under jc:intel:ocr_cache:{tenant_id}:{content_hash} (30d TTL)
// so identical documents are not re-OCRed (BUG-17 fix,
// valkey-keys-catalog Section INTEL, tenant-scoped per ADR-112 Amendment 2).
type OCRFetcher struct {
	tesseractBin string
	pdftoppmBin  string
	rdb          valkeycompat.Cmdable
	tenantID     pgtype.UUID
}

// NewOCRFetcher returns an OCRFetcher using the default binary names and
// no caching (back-compat with existing call sites).
func NewOCRFetcher() *OCRFetcher {
	return &OCRFetcher{tesseractBin: "tesseract", pdftoppmBin: "pdftoppm"}
}

// NewOCRFetcherWithCache returns an OCRFetcher with Valkey-backed OCR
// result caching. The content hash of the PDF body is used as the cache
// key so identical documents skip the expensive OCR step (BUG-17 fix).
// The cache key is tenant-scoped per ADR-112 Amendment 2.
func NewOCRFetcherWithCache(rdb valkeycompat.Cmdable, tenantID pgtype.UUID) *OCRFetcher {
	return &OCRFetcher{tesseractBin: "tesseract", pdftoppmBin: "pdftoppm", rdb: rdb, tenantID: tenantID}
}

// Available returns true if both tesseract and pdftoppm are installed.
func (f *OCRFetcher) Available() bool {
	if _, err := exec.LookPath(f.tesseractBin); err != nil {
		return false
	}
	if _, err := exec.LookPath(f.pdftoppmBin); err != nil {
		return false
	}
	return true
}

// ExtractText writes the PDF body to a temp file, converts each page to a PNG
// with pdftoppm, runs tesseract on each image, and concatenates the results.
// Returns an error if the required binaries are not installed.
//
// When a Valkey client is configured, the OCR result is cached under
// jc:intel:ocr_cache:{tenant_id}:{content_hash} (30d TTL) so identical
// documents skip the expensive OCR step on subsequent crawls (BUG-17 fix,
// tenant-scoped per ADR-112 Amendment 2).
func (f *OCRFetcher) ExtractText(body []byte) (string, error) {
	if !f.Available() {
		return "", fmt.Errorf("ocr: tesseract/pdftoppm not installed")
	}

	// Compute content hash for cache key.
	h := sha256.Sum256(body)
	contentHash := hex.EncodeToString(h[:])
	cacheKey := fmt.Sprintf("jc:intel:ocr_cache:%s:%s", f.tenantID.String(), contentHash)

	// Check cache first (BUG-17 fix).
	if f.rdb != nil {
		if cached, err := f.rdb.Get(context.Background(), cacheKey).Result(); err == nil {
			return cached, nil
		}
	}

	dir, err := os.MkdirTemp("", "japa-ocr-")
	if err != nil {
		return "", fmt.Errorf("ocr: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(pdfPath, body, 0o600); err != nil { // #nosec G306 -- temp dir is 0700, file is short-lived
		return "", fmt.Errorf("ocr: write pdf: %w", err)
	}

	// pdftoppm converts the PDF to numbered PNG images: prefix-1.png, prefix-2.png, ...
	imgPrefix := filepath.Join(dir, "page")
	// BUG-013 fix: cap the exec with a timeout so a large/scanned PDF
	// cannot hang the Asynq default queue indefinitely. A timeout kills
	// the process and returns an error; the worker retries then routes
	// the record to REVIEW_REQUIRED (spec §970 graceful degradation).
	ppmCtx, ppmCancel := context.WithTimeout(context.Background(), ocrExecTimeout)
	defer ppmCancel()
	ppmCmd := exec.CommandContext(ppmCtx, f.pdftoppmBin, "-png", "-r", "200", pdfPath, imgPrefix) // #nosec G204 -- binaries are hardcoded, paths are temp-dir scoped
	if err := ppmCmd.Run(); err != nil {
		return "", fmt.Errorf("ocr: pdftoppm: %w", err)
	}

	images, err := filepath.Glob(imgPrefix + "-*.png")
	if err != nil {
		return "", fmt.Errorf("ocr: glob images: %w", err)
	}
	if len(images) == 0 {
		return "", fmt.Errorf("ocr: pdftoppm produced no images")
	}
	sort.Strings(images)

	var b strings.Builder
	for _, img := range images {
		tessCtx, tessCancel := context.WithTimeout(context.Background(), ocrExecTimeout)
		out, err := exec.CommandContext(tessCtx, f.tesseractBin, img, "stdout", "-l", "eng").Output() // #nosec G204 -- tesseract bin is hardcoded, img path is temp-dir scoped
		tessCancel()
		if err != nil {
			// Skip a failed page (incl. timeout) rather than aborting the whole document.
			continue
		}
		b.Write(bytes.TrimSpace(out))
		b.WriteString("\n")
	}
	result := strings.TrimSpace(b.String())

	// Cache the OCR result (BUG-17 fix).
	if f.rdb != nil && result != "" {
		f.rdb.Set(context.Background(), cacheKey, result, ocrCacheTTL)
	}

	return result, nil
}
