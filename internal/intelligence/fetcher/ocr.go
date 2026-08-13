package fetcher

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// OCRFetcher runs tesseract (via pdftoppm for image conversion) to extract
// text from scanned PDFs. It is a fallback used by PDFFetcher when the
// embedded text layer is empty. Callers should check Available() before use
// and route to REVIEW_REQUIRED when tesseract is absent.
type OCRFetcher struct {
	tesseractBin string
	pdftoppmBin  string
}

// NewOCRFetcher returns an OCRFetcher using the default binary names.
func NewOCRFetcher() *OCRFetcher {
	return &OCRFetcher{tesseractBin: "tesseract", pdftoppmBin: "pdftoppm"}
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
func (f *OCRFetcher) ExtractText(body []byte) (string, error) {
	if !f.Available() {
		return "", fmt.Errorf("ocr: tesseract/pdftoppm not installed")
	}

	dir, err := os.MkdirTemp("", "japa-ocr-")
	if err != nil {
		return "", fmt.Errorf("ocr: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(pdfPath, body, 0o644); err != nil {
		return "", fmt.Errorf("ocr: write pdf: %w", err)
	}

	// pdftoppm converts the PDF to numbered PNG images: prefix-1.png, prefix-2.png, ...
	imgPrefix := filepath.Join(dir, "page")
	ppmCmd := exec.Command(f.pdftoppmBin, "-png", "-r", "200", pdfPath, imgPrefix)
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
		out, err := exec.Command(f.tesseractBin, img, "stdout", "-l", "eng").Output()
		if err != nil {
			// Skip a failed page rather than aborting the whole document.
			continue
		}
		b.Write(bytes.TrimSpace(out))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}
