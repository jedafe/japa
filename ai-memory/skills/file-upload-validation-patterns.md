# Skill: File Upload Validation & Malware Scanning Patterns

See ADR-023 for the why. This file is the how.

## Server-side MIME detection (never trust the client)
```go
func DetectAndValidateMIME(file io.Reader, allowed []string) (string, error) {
    buf := make([]byte, 512)
    n, err := file.Read(buf)
    if err != nil && err != io.EOF {
        return "", err
    }
    detected := http.DetectContentType(buf[:n]) // content inspection, not the Content-Type header
    for _, a := range allowed {
        if detected == a {
            return detected, nil
        }
    }
    return "", fmt.Errorf("rejected upload: detected type %s not in allowed list", detected)
}
```

## Per-context size limits
```go
var uploadLimits = map[string]int64{
    "avatar":      5 * 1024 * 1024,   // 5MB
    "post_image":  10 * 1024 * 1024,  // 10MB
    "document":    25 * 1024 * 1024,  // 25MB
}

func ValidateUploadSize(context string, size int64) error {
    limit, ok := uploadLimits[context]
    if !ok {
        return fmt.Errorf("unknown upload context: %s", context)
    }
    if size > limit {
        return fmt.Errorf("upload exceeds %d byte limit for context %s", limit, context)
    }
    return nil
}
```

## Upload pipeline — scan blocks persistence, never the other way around
```go
func HandleUpload(ctx context.Context, file UploadedFile) (*MediaRecord, error) {
    if err := ValidateUploadSize(file.Context, file.Size); err != nil {
        return nil, err
    }
    if _, err := DetectAndValidateMIME(file.Reader, allowedTypesFor(file.Context)); err != nil {
        return nil, err
    }
    // Stage to a temp location, NOT MinIO yet — media_scan must clear first
    tempPath := stageTempFile(file)
    record := createPendingMediaRecord(ctx, tempPath, file.Context)
    asynqClient.Enqueue(tasks.NewMediaScanTask(record.ID), asynq.Queue("default"))
    return record, nil // status: "pending_scan" until the task clears or flags it
}

// media_scan task handler
func HandleMediaScan(ctx context.Context, t *asynq.Task) error {
    var p MediaScanPayload
    json.Unmarshal(t.Payload(), &p)
    clean, err := clamavClient.ScanFile(p.TempPath)
    if err != nil {
        return fmt.Errorf("scan failed for media %s: %w", p.MediaID, err)
    }
    if !clean {
        rejectUpload(ctx, p.MediaID) // notify uploader, delete temp file, never persist
        return nil
    }
    persistToMinIO(ctx, p.MediaID, p.TempPath) // only now does it become servable
    return nil
}
```
