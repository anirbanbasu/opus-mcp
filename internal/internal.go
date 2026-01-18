package internal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/sethvargo/go-envconfig"
)

type HTTPClientConfig struct {
	HTTPProxyConfig    *HTTPProxyConfig
	TLSSecureConfig    *TLSSecureConfig
	MaxIdleConnections int                `env:"OPUS_MCP_HTTP_MAX_IDLE_CONNECTIONS,default=10"`
	HTTPTimeoutConfig  *HTTPTimeoutConfig `env:",prefix=OPUS_MCP_"`
}

type HTTPTimeoutConfig struct {
	IdleConnectionTimeout time.Duration `env:"HTTP_IDLE_CONNECTION_TIMEOUT,default=30s"`
	TLSHandshakeTimeout   time.Duration `env:"HTTP_TLS_HANDSHAKE_TIMEOUT,default=10s"`
	ClientTimeout         time.Duration `env:"HTTP_CLIENT_TIMEOUT,default=30s"`
}

type HTTPProxyConfig struct {
	HttpProxy  string `env:"http_proxy,default="`
	HttpsProxy string `env:"https_proxy,default="`
	NoProxy    string `env:"no_proxy,default="`
}

func AliasedLookuper(l envconfig.Lookuper) envconfig.Lookuper {
	aliases := map[string]string{
		"http_proxy":  "HTTP_PROXY",
		"https_proxy": "HTTPS_PROXY",
		"no_proxy":    "NO_PROXY",
	}
	return envconfig.LookuperFunc(func(s string) (string, bool) {
		if alias, ok := aliases[s]; ok {
			return l.Lookup(alias)
		} else {
			return l.Lookup(s)
		}
	})
}

type TLSSecureConfig struct {
	SslCertFile      string `env:"SSL_CERT_FILE,default="`
	RequestsCaBundle string `env:"REQUESTS_CA_BUNDLE,default="`
	CurlCaBundle     string `env:"CURL_CA_BUNDLE,default="`
	// InsecureSkipVerify indicates whether to skip TLS certificate verification.
	// ⚠️ WARNING: Disabling verification is insecure and should only be used in development/testing.
	InsecureSkipVerify bool `env:"OPUS_MCP_INSECURE_SKIP_VERIFY,default=false"`
}

// CreateConfiguredHTTPClient creates an HTTP client with proxy support and custom TLS configuration.
// It respects standard proxy environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY) and
// supports custom CA certificates via SSL_CERT_FILE or REQUESTS_CA_BUNDLE environment variables.
// If OPUS_MCP_INSECURE_SKIP_VERIFY=true is set, certificate verification will be disabled (⚠️ INSECURE).
func CreateConfiguredHTTPClient() (*http.Client, error) {
	// Setup TLS config
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Enforce minimum TLS 1.2
	}

	ctx := context.Background()
	var config HTTPClientConfig
	envc := envconfig.Config{
		Target:   &config,
		Lookuper: AliasedLookuper(envconfig.OsLookuper()),
	}
	if err := envconfig.ProcessWith(ctx, &envc); err != nil {
		slog.Error("Failed to process HTTP secure configuration from environment", "error", err)
		return nil, err
	}

	// Load custom CAs if specified
	if customCA := LoadCustomCABundle(config.TLSSecureConfig); customCA != nil {
		tlsConfig.RootCAs = customCA
	}

	// Check for insecure mode
	if config.TLSSecureConfig != nil && config.TLSSecureConfig.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
		slog.Warn("🚨 HTTP TLS certificate verification is DISABLED")
	}

	// Create transport with proxy support
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        config.MaxIdleConnections,
		IdleConnTimeout:     config.HTTPTimeoutConfig.IdleConnectionTimeout,
		TLSHandshakeTimeout: config.HTTPTimeoutConfig.TLSHandshakeTimeout,
	}

	// Log proxy configuration if set (with credentials removed)
	if config.HTTPProxyConfig.HttpProxy != "" {
		slog.Info("Using HTTP proxy", "proxy", SanitizeProxyURL(config.HTTPProxyConfig.HttpProxy))
	}

	if config.HTTPProxyConfig.HttpsProxy != "" {
		slog.Info("Using HTTPS proxy", "proxy", SanitizeProxyURL(config.HTTPProxyConfig.HttpsProxy))
	}

	return &http.Client{
		Transport: transport,
		Timeout:   config.HTTPTimeoutConfig.ClientTimeout,
	}, nil
}

// LoadCustomCABundle loads custom CA certificates from environment-specified paths.
// It checks SSL_CERT_FILE, REQUESTS_CA_BUNDLE, and CURL_CA_BUNDLE in that order.
// Returns a cert pool with system CAs plus any custom CAs found, or nil if none specified.
func LoadCustomCABundle(tlsConfig *TLSSecureConfig) *x509.CertPool {
	if tlsConfig == nil {
		return nil
	}
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
		{"SSL_CERT_FILE", tlsConfig.SslCertFile},
		{"REQUESTS_CA_BUNDLE", tlsConfig.RequestsCaBundle},
		{"CURL_CA_BUNDLE", tlsConfig.CurlCaBundle},
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
