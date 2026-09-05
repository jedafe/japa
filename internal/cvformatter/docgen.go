// Package cvformatter — docgen.go generates valid DOCX and PDF files from
// the AI-generated CV text. The download endpoint uses these functions to
// produce actual document files (not plain text with a wrong extension).
//
// DOCX is generated using archive/zip + encoding/xml (no external dependency).
// PDF is generated using github.com/jung-kurt/gofpdf.
package cvformatter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// GenerateDOCX creates a valid .docx file from plain text. The DOCX format
// is a ZIP archive containing XML files; we generate the minimal set:
//   - [Content_Types].xml
//   - _rels/.rels
//   - word/document.xml
//   - word/_rels/document.xml.rels
//
// Each paragraph in the input text becomes a <w:p> element with <w:t> runs.
func GenerateDOCX(text string) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// [Content_Types].xml
	if err := writeZipFile(zw, "[Content_Types].xml", contentTypesXML); err != nil {
		return nil, fmt.Errorf("docx: write content types: %w", err)
	}

	// _rels/.rels
	if err := writeZipFile(zw, "_rels/.rels", relsXML); err != nil {
		return nil, fmt.Errorf("docx: write rels: %w", err)
	}

	// word/_rels/document.xml.rels
	if err := writeZipFile(zw, "word/_rels/document.xml.rels", docRelsXML); err != nil {
		return nil, fmt.Errorf("docx: write doc rels: %w", err)
	}

	// word/document.xml — the main content.
	docXML := buildDocumentXML(text)
	if err := writeZipFile(zw, "word/document.xml", docXML); err != nil {
		return nil, fmt.Errorf("docx: write document: %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("docx: close zip: %w", err)
	}

	return buf.Bytes(), nil
}

// GeneratePDF creates a valid .pdf file from plain text using gofpdf.
// The text is rendered with a standard font (Helvetica), 11pt body, with
// basic paragraph handling.
func GeneratePDF(text string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(25, 25, 25)
	pdf.SetAutoPageBreak(true, 25)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)

	for _, line := range strings.Split(text, "\n") {
		// gofpdf MultiCell handles wrapping and page breaks automatically.
		// Trim trailing whitespace but preserve empty lines as spacing.
		trimmed := strings.TrimRight(line, "\r\t ")
		if trimmed == "" {
			pdf.Ln(5)
			continue
		}

		pdf.MultiCell(0, 6, trimmed, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf: output: %w", err)
	}

	return buf.Bytes(), nil
}

// writeZipFile writes a single file into a zip.Writer.
func writeZipFile(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(content))

	return err
}

// buildDocumentXML converts plain text into OOXML document.xml content.
// Each line becomes a paragraph (<w:p>); text within a paragraph is escaped
// and wrapped in a <w:r><w:t> run.
func buildDocumentXML(text string) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)

	for _, line := range strings.Split(text, "\n") {
		b.WriteString("<w:p>")

		trimmed := strings.TrimRight(line, "\r\t ")
		if trimmed != "" {
			b.WriteString("<w:r><w:t xml:space=\"preserve\">")
			b.WriteString(escapeXML(trimmed))
			b.WriteString("</w:t></w:r>")
		}

		b.WriteString("</w:p>")
	}

	b.WriteString("</w:document>")

	return b.String()
}

// escapeXML escapes the five XML special characters.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)

	return r.Replace(s)
}

// Minimal DOCX XML constants.
const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`
