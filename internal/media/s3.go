package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// S3Client provides S3-compatible presigned URL generation for SeaweedFS.
// It reads configuration from environment variables:
//   - JC_S3_ENDPOINT: S3 endpoint (e.g., "http://localhost:8333")
//   - JC_S3_ACCESS_KEY: S3 access key
//   - JC_S3_SECRET_KEY: S3 secret key
//   - JC_S3_REGION: S3 region (default "us-east-1")
type S3Client struct {
	endpoint   string
	accessKey  string
	secretKey  string
	region     string
	bucket     string
	httpClient *http.Client
}

// NewS3Client creates an S3 client from environment variables.
// Returns nil if required env vars are missing (graceful degradation).
func NewS3Client() *S3Client {
	endpoint := os.Getenv("JC_S3_ENDPOINT")
	accessKey := os.Getenv("JC_S3_ACCESS_KEY")
	secretKey := os.Getenv("JC_S3_SECRET_KEY")

	if endpoint == "" || accessKey == "" || secretKey == "" {
		slog.Warn("s3 client not configured, presigned URLs will use fallback paths",
			"endpoint_set", endpoint != "",
			"access_key_set", accessKey != "",
			"secret_key_set", secretKey != "")

		return nil
	}

	region := os.Getenv("JC_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}

	bucket := os.Getenv("JC_S3_BUCKET")
	if bucket == "" {
		bucket = DefaultBucket
	}

	return &S3Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		accessKey:  accessKey,
		secretKey:  secretKey,
		region:     region,
		bucket:     bucket,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
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
