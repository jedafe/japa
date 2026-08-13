# Intelligence System

## OCR Dependencies

The JapaConnect intelligence system includes an OCR fetcher adapter that
provides a fallback path for scanned PDFs (image-based documents that cannot
be parsed as text). This adapter relies on two system-level tools:

- **tesseract-ocr** — the OCR engine that extracts text from images.
- **tesseract-ocr-data-eng** — English language data for tesseract.
- **poppler-utils** — provides `pdftoppm`, which rasterizes PDF pages into
  images that tesseract can read.

### Installation

These dependencies are installed in the **runtime image** (Stage 2) of the
multi-stage `Dockerfile`, not the build stage, because they are needed at
runtime rather than compile time:

```dockerfile
RUN apk --no-cache add ca-certificates tesseract-ocr tesseract-ocr-data-eng poppler-utils
```

### Graceful Degradation

The intelligence system is designed to degrade gracefully when tesseract is
not present in the host environment. If the OCR tools are unavailable:

- OCR-dependent records are **not** dropped or silently discarded.
- Instead, they are routed to the `REVIEW_REQUIRED` queue so a human reviewer
  can handle them.
- This means a missing tesseract binary is **not a hard failure** — the API
  continues to serve normally, and only scanned-PDF ingestion is affected.

In production, the Dockerfile installs these dependencies so the full OCR
pipeline is available. In development or minimal environments where they are
intentionally omitted, the graceful-degradation path ensures the system keeps
running.
