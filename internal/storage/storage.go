package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"opus-mcp/internal"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig holds the configuration for S3-compatible storage connections
type MinIOConfig struct {
	Endpoint           string // S3 server endpoint (e.g., "play.min.io:9000")
	AccessKey          string // S3 access key
	SecretKey          string // S3 secret key
	UseSSL             bool   // Whether to use SSL/TLS
	InsecureSkipVerify bool   // Whether to skip TLS certificate verification (⚠️ INSECURE - use only for self-signed certificates in dev/test)
}

// createMinIOClient creates a configured S3 client with the given config
func createMinIOClient(config MinIOConfig) (*minio.Client, error) {
	minioOptions := &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
	}

	// Configure custom transport for insecure TLS if needed
	if config.InsecureSkipVerify {
		slog.Warn("🚨 SECURITY WARNING: TLS certificate verification is DISABLED for S3 connection (InsecureSkipVerify=true)")
		minioOptions.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	return minio.New(config.Endpoint, minioOptions)
}

// DownloadURLToMinIO downloads a file from an HTTP(s) URL and uploads it to an S3 bucket.
// It uses the CreateConfiguredHTTPClient function for the HTTP download to support proxy
// configurations and custom CA certificates.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - sourceURL: The HTTP(s) URL to download the file from
//   - config: S3 configuration (endpoint, credentials, SSL settings)
//   - bucketName: Target S3 bucket name
//   - objectName: Target object name in the bucket (file name/path)
//
// Returns an error if any step fails (download, upload, or S3 operations).
func DownloadURLToMinIO(ctx context.Context, sourceURL string, config MinIOConfig, bucketName, objectName string) error {
	// Validate inputs
	if sourceURL == "" {
		return fmt.Errorf("source URL cannot be empty")
	}
	if bucketName == "" {
		return fmt.Errorf("bucket name cannot be empty")
	}
	if objectName == "" {
		return fmt.Errorf("object name cannot be empty")
	}

	// Parse and validate the source URL
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s (only http and https are supported)", parsedURL.Scheme)
	}

	// Initialize MinIO client
	minioClient, err := createMinIOClient(config)
	if err != nil {
		return fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Check if bucket exists and is accessible
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket '%s' does not exist", bucketName)
	}

	slog.Info("Starting download from URL to S3 storage",
		"source_url", sourceURL,
		"bucket", bucketName,
		"object", objectName,
		"endpoint", config.Endpoint)

	// Download the file from the URL using the configured HTTP client
	httpClient := internal.CreateConfiguredHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	startTime := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	// Determine content type and size
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	contentLength := resp.ContentLength

	slog.Info("File download started",
		"content_type", contentType,
		"content_length", contentLength,
		"status_code", resp.StatusCode)

	// Upload to S3 using PutObject
	// PutObject automatically handles streaming the data
	uploadInfo, err := minioClient.PutObject(ctx, bucketName, objectName, resp.Body, contentLength, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"source-url":    sourceURL,
			"download-date": time.Now().Format(time.RFC3339),
			"original-name": filepath.Base(parsedURL.Path),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	duration := time.Since(startTime)
	slog.Info("Successfully uploaded file to S3 storage",
		"bucket", bucketName,
		"object", objectName,
		"size", uploadInfo.Size,
		"etag", uploadInfo.ETag,
		"duration", duration,
		"version_id", uploadInfo.VersionID)

	return nil
}

// DownloadURLToMinIOStream is a streaming variant that doesn't require knowing the content length upfront.
// This is useful when the server doesn't provide Content-Length header or for very large files.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - sourceURL: The HTTP(s) URL to download the file from
//   - config: S3 configuration (endpoint, credentials, SSL settings)
//   - bucketName: Target S3 bucket name
//   - objectName: Target object name in the bucket (file name/path)
//
// Returns an error if any step fails (download, upload, or S3 operations).
func DownloadURLToMinIOStream(ctx context.Context, sourceURL string, config MinIOConfig, bucketName, objectName string) error {
	// Validate inputs
	if sourceURL == "" {
		return fmt.Errorf("source URL cannot be empty")
	}
	if bucketName == "" {
		return fmt.Errorf("bucket name cannot be empty")
	}
	if objectName == "" {
		return fmt.Errorf("object name cannot be empty")
	}

	// Parse and validate the source URL
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s (only http and https are supported)", parsedURL.Scheme)
	}

	// Initialize S3 client
	minioClient, err := createMinIOClient(config)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Check if bucket exists and is accessible
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket '%s' does not exist", bucketName)
	}

	slog.Info("Starting streaming download from URL to S3 storage",
		"source_url", sourceURL,
		"bucket", bucketName,
		"object", objectName,
		"endpoint", config.Endpoint)

	// Download the file from the URL using the configured HTTP client
	httpClient := internal.CreateConfiguredHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	startTime := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	// Determine content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	slog.Info("File download started (streaming mode)",
		"content_type", contentType,
		"status_code", resp.StatusCode)

	// Upload to S3 using PutObject with -1 for unknown size (streaming mode)
	uploadInfo, err := minioClient.PutObject(ctx, bucketName, objectName, resp.Body, -1, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"source-url":    sourceURL,
			"download-date": time.Now().Format(time.RFC3339),
			"original-name": filepath.Base(parsedURL.Path),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	duration := time.Since(startTime)
	slog.Info("Successfully uploaded file to S3 storage (streaming mode)",
		"bucket", bucketName,
		"object", objectName,
		"size", uploadInfo.Size,
		"etag", uploadInfo.ETag,
		"duration", duration,
		"version_id", uploadInfo.VersionID)

	return nil
}

// DownloadURLToMinIOWithProgress downloads a file from an HTTP(s) URL and uploads it to S3 storage
// with progress reporting. This is useful for large files where you want to track upload progress.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - sourceURL: The HTTP(s) URL to download the file from
//   - config: S3 configuration (endpoint, credentials, SSL settings)
//   - bucketName: Target S3 bucket name
//   - objectName: Target object name in the bucket (file name/path)
//   - progressFunc: Optional callback function that receives progress updates (bytesTransferred, totalBytes)
//
// Returns an error if any step fails (download, upload, or S3 operations).
func DownloadURLToMinIOWithProgress(ctx context.Context, sourceURL string, config MinIOConfig, bucketName, objectName string, progressFunc func(bytesTransferred, totalBytes int64)) error {
	// Validate inputs
	if sourceURL == "" {
		return fmt.Errorf("source URL cannot be empty")
	}
	if bucketName == "" {
		return fmt.Errorf("bucket name cannot be empty")
	}
	if objectName == "" {
		return fmt.Errorf("object name cannot be empty")
	}

	// Parse and validate the source URL
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s (only http and https are supported)", parsedURL.Scheme)
	}

	// Initialize S3 client
	minioClient, err := createMinIOClient(config)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Check if bucket exists and is accessible
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket '%s' does not exist", bucketName)
	}

	slog.Info("Starting download from URL to S3 storage with progress tracking",
		"source_url", sourceURL,
		"bucket", bucketName,
		"object", objectName,
		"endpoint", config.Endpoint)

	// Download the file from the URL using the configured HTTP client
	httpClient := internal.CreateConfiguredHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	startTime := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	// Determine content type and size
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	contentLength := resp.ContentLength

	slog.Info("File download started with progress tracking",
		"content_type", contentType,
		"content_length", contentLength,
		"status_code", resp.StatusCode)

	// Wrap the reader with progress tracking if callback provided
	var reader io.Reader = resp.Body
	if progressFunc != nil {
		reader = &progressReader{
			reader:       resp.Body,
			totalBytes:   contentLength,
			progressFunc: progressFunc,
		}
	}

	// Upload to S3 using PutObject
	uploadInfo, err := minioClient.PutObject(ctx, bucketName, objectName, reader, contentLength, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"source-url":    sourceURL,
			"download-date": time.Now().Format(time.RFC3339),
			"original-name": filepath.Base(parsedURL.Path),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	duration := time.Since(startTime)
	slog.Info("Successfully uploaded file to S3 storage with progress tracking",
		"bucket", bucketName,
		"object", objectName,
		"size", uploadInfo.Size,
		"etag", uploadInfo.ETag,
		"duration", duration,
		"version_id", uploadInfo.VersionID)

	return nil
}

// progressReader wraps an io.Reader to provide progress callbacks
type progressReader struct {
	reader           io.Reader
	bytesTransferred int64
	totalBytes       int64
	progressFunc     func(bytesTransferred, totalBytes int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.bytesTransferred += int64(n)

	if pr.progressFunc != nil && n > 0 {
		pr.progressFunc(pr.bytesTransferred, pr.totalBytes)
	}

	return n, err
}
