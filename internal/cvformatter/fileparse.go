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
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxUploadBytes is the maximum accepted file size for CV uploads (10 MB).
const MaxUploadBytes = 10 << 20

// AllowedMimeTypes lists the MIME types accepted for CV file uploads.
var AllowedMimeTypes = map[string]bool{
	"application/pdf":                                                                  true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":           true,
}

// ExtractTextFromPDF pulls text from a PDF byte slice using the
// ledongthuc/pdf library (already in go.mod for the intelligence fetcher).
func ExtractTextFromPDF(body []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("pdf: new reader: %w", err)
	}
	var b strings.Builder
	totalPage := r.NumPage()
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
func ExtractTextFromDOCX(body []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("docx: open zip: %w", err)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("docx: word/document.xml not found in archive")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("docx: open document.xml: %w", err)
	}
	defer rc.Close()
	xmlBody, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("docx: read document.xml: %w", err)
	}
	return extractDocxText(xmlBody), nil
}

// docxText represents the minimal OOXML structure we need to parse.
// We walk the XML tree and collect text from <w:t> elements, inserting
// line breaks for <w:p> (paragraph) and <w:br> (break) elements.
type docxText struct{}

// extractDocxText parses the document.xml body and extracts text from
// <w:t> elements, preserving paragraph boundaries.
func extractDocxText(body []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var b strings.Builder
	var inT bool
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
