package internal

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

// CreateConfiguredHTTPClient creates an HTTP client with proxy support and custom TLS configuration.
// It respects standard proxy environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY) and
// supports custom CA certificates via SSL_CERT_FILE or REQUESTS_CA_BUNDLE environment variables.
// If OPUS_MCP_INSECURE_SKIP_VERIFY=true is set, certificate verification will be disabled (⚠️ INSECURE).
func CreateConfiguredHTTPClient() *http.Client {
	// Setup TLS config
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Enforce minimum TLS 1.2
	}

	// Load custom CAs if specified
	if customCA := LoadCustomCABundle(); customCA != nil {
		tlsConfig.RootCAs = customCA
	}

	// Check for insecure mode
	if ShouldSkipTLSVerification() {
		tlsConfig.InsecureSkipVerify = true
		slog.Warn("🚨 SECURITY WARNING: TLS certificate verification is DISABLED (InsecureSkipVerify=true)")
	}

	// Create transport with proxy support
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Log proxy configuration if set (with credentials removed)
	if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
		slog.Info("Using HTTP proxy", "proxy", SanitizeProxyURL(proxy))
	} else if proxy := os.Getenv("http_proxy"); proxy != "" {
		slog.Info("Using HTTP proxy", "proxy", SanitizeProxyURL(proxy))
	}

	if proxy := os.Getenv("HTTPS_PROXY"); proxy != "" {
		slog.Info("Using HTTPS proxy", "proxy", SanitizeProxyURL(proxy))
	} else if proxy := os.Getenv("https_proxy"); proxy != "" {
		slog.Info("Using HTTPS proxy", "proxy", SanitizeProxyURL(proxy))
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Overall request timeout
	}
}

// LoadCustomCABundle loads custom CA certificates from environment-specified paths.
// It checks SSL_CERT_FILE, REQUESTS_CA_BUNDLE, and CURL_CA_BUNDLE in that order.
// Returns a cert pool with system CAs plus any custom CAs found, or nil if none specified.
func LoadCustomCABundle() *x509.CertPool {
	// Start with system's trusted CAs
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		slog.Warn("Failed to load system cert pool, creating new one", "error", err)
		rootCAs = x509.NewCertPool()
	}

	// Check environment variables for custom CA paths
	caPaths := []struct {
		envVar string
		path   string
	}{
		{"SSL_CERT_FILE", os.Getenv("SSL_CERT_FILE")},
		{"REQUESTS_CA_BUNDLE", os.Getenv("REQUESTS_CA_BUNDLE")},
		{"CURL_CA_BUNDLE", os.Getenv("CURL_CA_BUNDLE")},
	}

	loadedAny := false
	for _, ca := range caPaths {
		if ca.path != "" {
			if caCert, err := os.ReadFile(ca.path); err == nil {
				if rootCAs.AppendCertsFromPEM(caCert) {
					slog.Info("Loaded custom CA certificate", "env_var", ca.envVar, "path", ca.path)
					loadedAny = true
				} else {
					slog.Warn("Failed to parse CA certificate", "env_var", ca.envVar, "path", ca.path)
				}
			} else {
				slog.Warn("Failed to load CA certificate file", "env_var", ca.envVar, "path", ca.path, "error", err)
			}
		}
	}

	if loadedAny {
		return rootCAs
	}
	return nil // Use default system CAs
}

// ShouldSkipTLSVerification checks if TLS certificate verification should be disabled.
// Returns true only if OPUS_MCP_INSECURE_SKIP_VERIFY environment variable is explicitly set to "true".
// ⚠️ WARNING: Disabling verification is insecure and should only be used in development/testing.
func ShouldSkipTLSVerification() bool {
	return os.Getenv("OPUS_MCP_INSECURE_SKIP_VERIFY") == "true"
}

// SanitizeProxyURL removes username and password from a proxy URL before logging.
// This prevents credentials from being exposed in logs.
func SanitizeProxyURL(proxyURL string) string {
	if proxyURL == "" {
		return ""
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		// If we can't parse it, return a safe placeholder
		return "<invalid-url>"
	}

	// Remove user info if present
	if parsed.User != nil {
		parsed.User = url.UserPassword("***", "***")
	}

	return parsed.String()
}
