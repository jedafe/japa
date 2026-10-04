package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// S3Client provides S3-compatible presigned URL generation for SeaweedFS.
// Configuration is supplied via S3Config (loaded by internal/config from
// JC_S3_* environment variables, Rule 3 — no ad-hoc os.Getenv in callers).
type S3Client struct {
	endpoint   string
	accessKey  string
	secretKey  string
	region     string
	bucket     string
	httpClient *http.Client
}

// S3Config holds the S3-compatible storage configuration loaded by
// internal/config from JC_S3_* environment variables. Zero values trigger
// graceful degradation (NewS3ClientFromConfig returns nil).
type S3Config struct {
	Endpoint  string // JC_S3_ENDPOINT — S3 endpoint (e.g., "http://localhost:8333")
	AccessKey string // JC_S3_ACCESS_KEY — S3 access key
	SecretKey string // JC_S3_SECRET_KEY — S3 secret key
	Region    string // JC_S3_REGION — S3 region (default "us-east-1")
	Bucket    string // JC_S3_BUCKET — bucket name (default DefaultBucket)
}

// NewS3ClientFromConfig creates an S3 client from an S3Config (loaded by
// internal/config). Returns nil if Endpoint, AccessKey, or SecretKey are
// missing (graceful degradation — callers fall back to local serve paths).
// Empty Region defaults to "us-east-1"; empty Bucket defaults to DefaultBucket.
func NewS3ClientFromConfig(c S3Config) *S3Client {
	if c.Endpoint == "" || c.AccessKey == "" || c.SecretKey == "" {
		slog.Warn("s3 client not configured, presigned URLs will use fallback paths",
			"endpoint_set", c.Endpoint != "",
			"access_key_set", c.AccessKey != "",
			"secret_key_set", c.SecretKey != "")

		return nil
	}

	region := c.Region
	if region == "" {
		region = "us-east-1"
	}

	bucket := c.Bucket
	if bucket == "" {
		bucket = DefaultBucket
	}

	return &S3Client{
		endpoint:   strings.TrimRight(c.Endpoint, "/"),
		accessKey:  c.AccessKey,
		secretKey:  c.SecretKey,
		region:     region,
		bucket:     bucket,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewS3Client creates an S3 client from environment variables.
// Returns nil if required env vars are missing (graceful degradation).
//
// Deprecated: prefer NewS3ClientFromConfig with a config.Config loaded by
// internal/config (Rule 3). This wrapper is retained for backward
// compatibility with callers that have not yet been wired with config
// injection (e.g. standalone tools, tests).
func NewS3Client() *S3Client {
	return NewS3ClientFromConfig(S3Config{
		Endpoint:  os.Getenv("JC_S3_ENDPOINT"),
		AccessKey: os.Getenv("JC_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("JC_S3_SECRET_KEY"),
		Region:    os.Getenv("JC_S3_REGION"),
		Bucket:    os.Getenv("JC_S3_BUCKET"),
	})
}

// PresignPut generates a presigned PUT URL for uploading a file to S3.
// The URL is valid for the specified duration (default 15 minutes).
func (c *S3Client) PresignPut(_ context.Context, storagePath string, expires time.Duration) (string, error) {
	if c == nil {
		return "", errors.New("s3 client not configured")
	}

	if expires <= 0 {
		expires = 15 * time.Minute
	}

	expiresAt := time.Now().Add(expires)

	// S3 Signature Version 4 for PUT presigned URL
	// Reference: https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
	dateStr := expiresAt.UTC().Format("20060102")
	datetimeStr := expiresAt.UTC().Format("20060102T150405Z")

	// Canonical request
	canonicalURI := "/" + c.bucket + "/" + storagePath
	canonicalQueryString := fmt.Sprintf(
		"X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=%s%%2F%s%%2F%s%%2Fs3%%2Faws4_request&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host",
		c.accessKey, dateStr, c.region, datetimeStr, int(expires.Seconds()),
	)

	// The endpoint URL (without query string)
	requestURL := fmt.Sprintf("%s%s", c.endpoint, canonicalURI)

	// String to sign
	stringToSign := fmt.Sprintf("PUT\n%s\n%s\n%s\nhost\nUNSIGNED-PAYLOAD",
		canonicalURI, canonicalQueryString, datetimeStr)

	// Signing key
	signingKey := c.getSigningKey(dateStr)

	// Signature
	signature := hex.EncodeToString(c.hmacSHA256(signingKey, []byte(stringToSign)))

	// Final URL
	presignedURL := fmt.Sprintf("%s?%s&X-Amz-Signature=%s",
		requestURL, canonicalQueryString, signature)

	return presignedURL, nil
}

// getSigningKey derives the AWS4 signing key.
func (c *S3Client) getSigningKey(date string) []byte {
	kDate := c.hmacSHA256([]byte("AWS4"+c.secretKey), []byte(date))
	kRegion := c.hmacSHA256(kDate, []byte(c.region))
	kService := c.hmacSHA256(kRegion, []byte("s3"))
	kSigning := c.hmacSHA256(kService, []byte("aws4_request"))

	return kSigning
}

// hmacSHA256 computes HMAC-SHA256.
func (c *S3Client) hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)

	return mac.Sum(nil)
}

// GetObjectURL returns the direct S3 URL for reading an object.
// This is used for internal operations like content inspection.
func (c *S3Client) GetObjectURL(storagePath string) string {
	if c == nil {
		return ""
	}

	return fmt.Sprintf("%s/%s/%s", c.endpoint, c.bucket, storagePath)
}

// PutObject uploads raw bytes to S3-compatible storage at the given storage
// path. Used by server-side ingestion tasks (e.g. university:fetch_logo) that
// download content from remote URLs and store it without a client-side
// presigned URL upload. Returns nil on success.
func (c *S3Client) PutObject(ctx context.Context, storagePath, mimeType string, body []byte) error {
	return c.putObject(ctx, c.bucket, storagePath, mimeType, body)
}

// putObject is the bucket-parameterized form of PutObject — used when the
// media row's storage_bucket may differ from the client default.
func (c *S3Client) putObject(ctx context.Context, bucket, storagePath, mimeType string, body []byte) error {
	if c == nil {
		return errors.New("s3 client not configured")
	}

	objectURL := fmt.Sprintf("%s/%s/%s", c.endpoint, bucket, storagePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating s3 put request: %w", err)
	}

	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}

	req.Header.Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing s3 put: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 put returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteObject removes an object from the given bucket. A 404 is treated
// as success — the object is already gone.
func (c *S3Client) DeleteObject(ctx context.Context, bucket, storagePath string) error {
	if c == nil {
		return errors.New("s3 client not configured")
	}

	objectURL := fmt.Sprintf("%s/%s/%s", c.endpoint, bucket, storagePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, objectURL, nil)
	if err != nil {
		return fmt.Errorf("creating s3 delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing s3 delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return fmt.Errorf("s3 delete returned %d", resp.StatusCode)
	}

	return nil
}
