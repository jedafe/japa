// Package cvformatter — fileparse.go extracts text from uploaded CV files
// (PDF and DOCX) so the AI formatter can process them. Uses the existing
// ledongthuc/pdf library for PDF and archive/zip + encoding/xml for DOCX.
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
package cvformatter

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxUploadBytes is the maximum accepted file size for CV uploads (10 MB).
const MaxUploadBytes = 10 << 20

const (
	// maxDocxFiles caps the number of entries in an uploaded DOCX/ZIP so a
	// many-tiny-files archive cannot exhaust file descriptors or memory.
	maxDocxFiles = 256
	// maxDocxUncompressed caps the total uncompressed bytes read from a
	// DOCX/ZIP (ZIP bomb guard alongside MaxUploadBytes on the wire size).
	maxDocxUncompressed = 64 << 20
	// maxDocxCompressionRatio caps the uncompressed:compressed size ratio —
	// classic ZIP bombs exceed 100:1.
	maxDocxCompressionRatio = 100
	// maxDocumentXMLBytes caps the single document.xml read before XML
	// parsing (entity-expansion + oversized-XML guard).
	maxDocumentXMLBytes = 16 << 20
	// maxXMLEntities caps resolved character/entity references while
	// walking document.xml (billion-laughs guard — encoding/xml has no
	// built-in entity budget).
	maxXMLEntities = 100000
	// maxPDFPages caps the pages processed per PDF (a crafted PDF can
	// declare hundreds of thousands of pages).
	maxPDFPages = 200
)

// AllowedMimeTypes lists the MIME types accepted for CV file uploads.
var AllowedMimeTypes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
}

// ExtractTextFromPDF pulls text from a PDF byte slice using the
// ledongthuc/pdf library (already in go.mod for the intelligence fetcher).
// Page processing is capped at maxPDFPages so a crafted PDF declaring a
// huge page count cannot pin CPU/memory.
func ExtractTextFromPDF(body []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("pdf: new reader: %w", err)
	}

	var b strings.Builder

	totalPage := r.NumPage()
	if totalPage > maxPDFPages {
		return "", fmt.Errorf("pdf: page count %d exceeds limit of %d", totalPage, maxPDFPages)
	}

	for i := 1; i <= totalPage; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		b.WriteString(text)
		b.WriteString("\n")
	}

	return b.String(), nil
}

// ExtractTextFromDOCX pulls text from a DOCX byte slice. DOCX is a ZIP
// archive containing word/document.xml; we parse the XML and extract
// text from <w:t> elements (the OOXML text run element).
//
// ZIP-bomb guards: entry count, total uncompressed size, and the
// uncompressed:compressed ratio are all capped before document.xml is
// read, and the XML read itself is capped at maxDocumentXMLBytes.
func ExtractTextFromDOCX(body []byte) (string, error) {
	if len(body) > MaxUploadBytes {
		return "", fmt.Errorf("docx: file size %d exceeds limit of %d bytes", len(body), MaxUploadBytes)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("docx: open zip: %w", err)
	}

	if len(zr.File) > maxDocxFiles {
		return "", fmt.Errorf("docx: archive holds %d files, exceeding the limit of %d", len(zr.File), maxDocxFiles)
	}

	var totalUncompressed uint64
	for _, f := range zr.File {
		totalUncompressed += f.UncompressedSize64
	}

	if totalUncompressed > maxDocxUncompressed {
		return "", fmt.Errorf("docx: uncompressed size %d exceeds limit of %d bytes", totalUncompressed, maxDocxUncompressed)
	}

	if len(body) > 0 && totalUncompressed/uint64(len(body)) > maxDocxCompressionRatio {
		return "", fmt.Errorf("docx: compression ratio exceeds %d:1 (possible zip bomb)", maxDocxCompressionRatio)
	}

	var docFile *zip.File

	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}

	if docFile == nil {
		return "", errors.New("docx: word/document.xml not found in archive")
	}

	if docFile.UncompressedSize64 > maxDocumentXMLBytes {
		return "", fmt.Errorf("docx: document.xml size %d exceeds limit of %d bytes", docFile.UncompressedSize64, maxDocumentXMLBytes)
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("docx: open document.xml: %w", err)
	}

	defer rc.Close()

	xmlBody, err := io.ReadAll(io.LimitReader(rc, maxDocumentXMLBytes+1))
	if err != nil {
		return "", fmt.Errorf("docx: read document.xml: %w", err)
	}

	if len(xmlBody) > maxDocumentXMLBytes {
		return "", fmt.Errorf("docx: document.xml exceeds limit of %d bytes", maxDocumentXMLBytes)
	}

	return extractDocxText(xmlBody), nil
}

// docxText represents the minimal OOXML structure we need to parse.
// We walk the XML tree and collect text from <w:t> elements, inserting
// line breaks for <w:p> (paragraph) and <w:br> (break) elements.
type docxText struct{}

// extractDocxText parses the document.xml body and extracts text from
// <w:t> elements, preserving paragraph boundaries.
//
// Entity-expansion guard: the decoder runs in Strict mode with entity
// expansion disabled (no Entity map — unknown entities resolve to empty,
// so a billion-laughs payload yields no text instead of gigabytes), and
// the total CharData/entity bytes consumed are capped at maxXMLEntities.
func extractDocxText(body []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = true
	decoder.Entity = nil
	decoder.CharsetReader = nil

	var b strings.Builder

	var inT bool

	entityBytes := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch v := tok.(type) {
		case xml.StartElement:
			local := v.Name.Local
			switch local {
			case "t":
				inT = true
			case "p", "br":
				// Paragraph or break — insert newline.
				b.WriteString("\n")
			}
		case xml.CharData:
			entityBytes += len(v)
			if entityBytes > maxXMLEntities {
				return strings.TrimSpace(b.String())
			}

			if inT {
				b.Write(v)
			}
		case xml.EndElement:
			if v.Name.Local == "t" {
				inT = false
			}
		}
	}

	return strings.TrimSpace(b.String())
}

// ExtractTextFromFile dispatches to the correct extractor based on MIME type.
// Returns the extracted text and an error if extraction fails or the MIME
// type is unsupported.
func ExtractTextFromFile(body []byte, mimeType string) (string, error) {
	switch mimeType {
	case "application/pdf":
		return ExtractTextFromPDF(body)
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ExtractTextFromDOCX(body)
	default:
		return "", fmt.Errorf("unsupported file type: %s (accepted: PDF, DOCX)", mimeType)
	}
}
