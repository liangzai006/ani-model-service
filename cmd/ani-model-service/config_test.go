package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateRequiredConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name: "all required vars set - MinIO mode",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":                "postgres://user:pass@localhost/db",
				"ANI_MINIO_ENDPOINT":              "minio:9000",
				"ANI_MINIO_ACCESS_KEY":            "minioadmin",
				"ANI_MINIO_SECRET_KEY":            "minioadmin",
				"ANI_IMPORT_JOB_IMAGE":            "ani-import:v1",
				"ANI_IMPORT_MINIO_SECRET":         "minio-secret",
				"ANI_IMPORT_KUBERNETES_NAMESPACE": "ani-model",
			},
			wantErr: false,
		},
		{
			name: "all required vars set - Storage gRPC mode",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":             "postgres://user:pass@localhost/db",
				"ANI_STORAGE_GRPC_ADDR":        "storage:8443",
				"ANI_STORAGE_GRPC_SERVER_NAME": "storage.ani.svc",
			},
			wantErr: false,
		},
		{
			name: "missing DATABASE_DSN",
			envVars: map[string]string{
				"ANI_MINIO_ENDPOINT":   "minio:9000",
				"ANI_MINIO_ACCESS_KEY": "minioadmin",
				"ANI_MINIO_SECRET_KEY": "minioadmin",
			},
			wantErr: true,
			errMsg:  "ANI_DATABASE_DSN",
		},
		{
			name: "missing storage backend",
			envVars: map[string]string{
				"ANI_DATABASE_DSN": "postgres://user:pass@localhost/db",
			},
			wantErr: true,
			errMsg:  "ANI_MINIO_ENDPOINT or ANI_STORAGE_GRPC_ADDR",
		},
		{
			name: "missing MinIO credentials - access key",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":                "postgres://user:pass@localhost/db",
				"ANI_MINIO_ENDPOINT":              "minio:9000",
				"ANI_MINIO_SECRET_KEY":            "minioadmin",
				"ANI_IMPORT_JOB_IMAGE":            "ani-import:v1",
				"ANI_IMPORT_MINIO_SECRET":         "minio-secret",
				"ANI_IMPORT_KUBERNETES_NAMESPACE": "ani-model",
			},
			wantErr: true,
			errMsg:  "ANI_MINIO_ACCESS_KEY",
		},
		{
			name: "missing MinIO credentials - secret key",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":                "postgres://user:pass@localhost/db",
				"ANI_MINIO_ENDPOINT":              "minio:9000",
				"ANI_MINIO_ACCESS_KEY":            "minioadmin",
				"ANI_IMPORT_JOB_IMAGE":            "ani-import:v1",
				"ANI_IMPORT_MINIO_SECRET":         "minio-secret",
				"ANI_IMPORT_KUBERNETES_NAMESPACE": "ani-model",
			},
			wantErr: true,
			errMsg:  "ANI_MINIO_SECRET_KEY",
		},
		{
			name: "missing import job image",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":                "postgres://user:pass@localhost/db",
				"ANI_MINIO_ENDPOINT":              "minio:9000",
				"ANI_MINIO_ACCESS_KEY":            "minioadmin",
				"ANI_MINIO_SECRET_KEY":            "minioadmin",
				"ANI_IMPORT_MINIO_SECRET":         "minio-secret",
				"ANI_IMPORT_KUBERNETES_NAMESPACE": "ani-model",
			},
			wantErr: true,
			errMsg:  "ANI_IMPORT_JOB_IMAGE",
		},
		{
			name: "missing storage gRPC server name",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":      "postgres://user:pass@localhost/db",
				"ANI_STORAGE_GRPC_ADDR": "storage:8443",
			},
			wantErr: true,
			errMsg:  "ANI_STORAGE_GRPC_SERVER_NAME",
		},
		{
			name: "partial configuration - MinIO endpoint but missing namespace",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":        "postgres://user:pass@localhost/db",
				"ANI_MINIO_ENDPOINT":      "minio:9000",
				"ANI_MINIO_ACCESS_KEY":    "minioadmin",
				"ANI_MINIO_SECRET_KEY":    "minioadmin",
				"ANI_IMPORT_JOB_IMAGE":    "ani-import:v1",
				"ANI_IMPORT_MINIO_SECRET": "minio-secret",
			},
			wantErr: true,
			errMsg:  "ANI_IMPORT_KUBERNETES_NAMESPACE",
		},
		{
			name: "whitespace-only values treated as missing",
			envVars: map[string]string{
				"ANI_DATABASE_DSN":     "   ",
				"ANI_MINIO_ENDPOINT":   "minio:9000",
				"ANI_MINIO_ACCESS_KEY": "minioadmin",
				"ANI_MINIO_SECRET_KEY": "minioadmin",
			},
			wantErr: true,
			errMsg:  "ANI_DATABASE_DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant environment variables
			clearEnv := []string{
				"ANI_DATABASE_DSN",
				"ANI_MINIO_ENDPOINT",
				"ANI_MINIO_ACCESS_KEY",
				"ANI_MINIO_SECRET_KEY",
				"ANI_IMPORT_JOB_IMAGE",
				"ANI_IMPORT_MINIO_SECRET",
				"ANI_IMPORT_KUBERNETES_NAMESPACE",
				"ANI_STORAGE_GRPC_ADDR",
				"ANI_STORAGE_GRPC_SERVER_NAME",
			}
			for _, key := range clearEnv {
				os.Unsetenv(key)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			err := validateRequiredConfig()

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateRequiredConfig() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateRequiredConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateRequiredConfig() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestBuildStorageTLSConfig(t *testing.T) {
	// Generate a test CA certificate
	testCA := generateTestCA(t)

	tests := []struct {
		name    string
		envVars map[string]string
		setup   func(*testing.T) string // Returns temp file path if needed
		wantErr bool
		errMsg  string
	}{
		{
			name:    "system cert pool (default)",
			envVars: map[string]string{},
			wantErr: false,
		},
		{
			name: "CA from file",
			envVars: map[string]string{
				"ANI_STORAGE_GRPC_SERVER_NAME": "storage.example.com",
			},
			setup: func(t *testing.T) string {
				tmpFile := filepath.Join(t.TempDir(), "ca.crt")
				if err := os.WriteFile(tmpFile, testCA, 0644); err != nil {
					t.Fatalf("failed to write test CA file: %v", err)
				}
				os.Setenv("ANI_STORAGE_GRPC_CA_CERT", tmpFile)
				return tmpFile
			},
			wantErr: false,
		},
		{
			name: "CA from environment variable",
			envVars: map[string]string{
				"ANI_STORAGE_GRPC_CA_CERT_PEM": string(testCA),
				"ANI_STORAGE_GRPC_SERVER_NAME": "storage.example.com",
			},
			wantErr: false,
		},
		{
			name:    "invalid CA cert from file",
			envVars: map[string]string{},
			setup: func(t *testing.T) string {
				tmpFile := filepath.Join(t.TempDir(), "invalid.crt")
				if err := os.WriteFile(tmpFile, []byte("not a valid certificate"), 0644); err != nil {
					t.Fatalf("failed to write invalid CA file: %v", err)
				}
				os.Setenv("ANI_STORAGE_GRPC_CA_CERT", tmpFile)
				return tmpFile
			},
			wantErr: true,
			errMsg:  "invalid CA certificate",
		},
		{
			name: "invalid CA cert from environment",
			envVars: map[string]string{
				"ANI_STORAGE_GRPC_CA_CERT_PEM": "not a valid certificate",
			},
			wantErr: true,
			errMsg:  "invalid CA certificate",
		},
		{
			name: "nonexistent CA file",
			envVars: map[string]string{
				"ANI_STORAGE_GRPC_CA_CERT": "/nonexistent/ca.crt",
			},
			wantErr: true,
			errMsg:  "load CA cert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Unsetenv("ANI_STORAGE_GRPC_CA_CERT")
			os.Unsetenv("ANI_STORAGE_GRPC_CA_CERT_PEM")
			os.Unsetenv("ANI_STORAGE_GRPC_SERVER_NAME")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			// Run setup if provided
			if tt.setup != nil {
				tmpPath := tt.setup(t)
				defer os.Unsetenv("ANI_STORAGE_GRPC_CA_CERT")
				if tmpPath != "" {
					defer os.Remove(tmpPath)
				}
			}

			config, err := buildStorageTLSConfig()

			if tt.wantErr {
				if err == nil {
					t.Errorf("buildStorageTLSConfig() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("buildStorageTLSConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("buildStorageTLSConfig() unexpected error = %v", err)
					return
				}
				if config == nil {
					t.Error("buildStorageTLSConfig() returned nil config")
					return
				}
				// Verify TLS 1.3 minimum version
				if config.MinVersion != tls.VersionTLS13 {
					t.Errorf("buildStorageTLSConfig() MinVersion = %v, want TLS 1.3 (%d)", config.MinVersion, tls.VersionTLS13)
				}
			}
		})
	}
}

func TestNewImporterHTTPClient(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		wantTimeout     time.Duration
		wantDialTimeout time.Duration
	}{
		{
			name:            "default timeouts",
			envVars:         map[string]string{},
			wantTimeout:     15 * time.Minute,
			wantDialTimeout: 30 * time.Second,
		},
		{
			name: "custom timeouts from env vars",
			envVars: map[string]string{
				"ANI_IMPORTER_HTTP_TIMEOUT": "5m",
				"ANI_IMPORTER_DIAL_TIMEOUT": "10s",
			},
			wantTimeout:     5 * time.Minute,
			wantDialTimeout: 10 * time.Second,
		},
		{
			name: "invalid timeout values - use defaults",
			envVars: map[string]string{
				"ANI_IMPORTER_HTTP_TIMEOUT": "invalid",
				"ANI_IMPORTER_DIAL_TIMEOUT": "not-a-duration",
			},
			wantTimeout:     15 * time.Minute,
			wantDialTimeout: 30 * time.Second,
		},
		{
			name: "negative timeout values - use defaults",
			envVars: map[string]string{
				"ANI_IMPORTER_HTTP_TIMEOUT": "-5m",
				"ANI_IMPORTER_DIAL_TIMEOUT": "-10s",
			},
			wantTimeout:     15 * time.Minute,
			wantDialTimeout: 30 * time.Second,
		},
		{
			name: "zero timeout values - use defaults",
			envVars: map[string]string{
				"ANI_IMPORTER_HTTP_TIMEOUT": "0",
				"ANI_IMPORTER_DIAL_TIMEOUT": "0s",
			},
			wantTimeout:     15 * time.Minute,
			wantDialTimeout: 30 * time.Second,
		},
		{
			name: "custom timeout only",
			envVars: map[string]string{
				"ANI_IMPORTER_HTTP_TIMEOUT": "30m",
			},
			wantTimeout:     30 * time.Minute,
			wantDialTimeout: 30 * time.Second,
		},
		{
			name: "custom dial timeout only",
			envVars: map[string]string{
				"ANI_IMPORTER_DIAL_TIMEOUT": "1m",
			},
			wantTimeout:     15 * time.Minute,
			wantDialTimeout: 1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Unsetenv("ANI_IMPORTER_HTTP_TIMEOUT")
			os.Unsetenv("ANI_IMPORTER_DIAL_TIMEOUT")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			client := newImporterHTTPClient()

			if client == nil {
				t.Fatal("newImporterHTTPClient() returned nil")
			}

			if client.Timeout != tt.wantTimeout {
				t.Errorf("newImporterHTTPClient() Timeout = %v, want %v", client.Timeout, tt.wantTimeout)
			}

			// Verify transport configuration
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("newImporterHTTPClient() Transport is not *http.Transport")
			}

			// Verify TLS handshake timeout
			if transport.TLSHandshakeTimeout != 10*time.Second {
				t.Errorf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, 10*time.Second)
			}

			// Verify response header timeout
			if transport.ResponseHeaderTimeout != 30*time.Second {
				t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, 30*time.Second)
			}

			// Verify connection pool settings
			if transport.MaxIdleConns != 100 {
				t.Errorf("MaxIdleConns = %v, want %v", transport.MaxIdleConns, 100)
			}

			if transport.MaxIdleConnsPerHost != 10 {
				t.Errorf("MaxIdleConnsPerHost = %v, want %v", transport.MaxIdleConnsPerHost, 10)
			}

			if transport.IdleConnTimeout != 90*time.Second {
				t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, 90*time.Second)
			}
		})
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func generateTestCA(t *testing.T) []byte {
	t.Helper()

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM
}
