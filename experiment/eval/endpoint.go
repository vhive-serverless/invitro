package eval

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const CanonicalMinioHost = "myminio-api.minio.10.200.3.4.sslip.io"

func NormalizeMinioEndpoint(raw string) (baseURL, clientEndpoint string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("MinIO endpoint is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
		return "", "", fmt.Errorf("MinIO endpoint must be an HTTP URL: %q", raw)
	}
	if parsed.Hostname() != CanonicalMinioHost {
		return "", "", fmt.Errorf("MinIO endpoint host %q is not canonical %q", parsed.Hostname(), CanonicalMinioHost)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", "", fmt.Errorf("MinIO base URL must not contain path %q", parsed.Path)
	}
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	clientEndpoint = net.JoinHostPort(parsed.Hostname(), port)
	return "http://" + clientEndpoint, clientEndpoint, nil
}

func MinioHealthURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/minio/health/live"
}
