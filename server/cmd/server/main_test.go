package main

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratePrivateKey(t *testing.T) {
	key, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("generatePrivateKey() error = %v", err)
	}

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("generatePrivateKey() returned %T, want *ecdsa.PrivateKey", key)
	}

	if ecKey.Curve.Params().Name != "P-256" {
		t.Errorf("generatePrivateKey() curve = %s, want P-256", ecKey.Curve.Params().Name)
	}
}

func TestGenerateCertificate(t *testing.T) {
	privateKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("generatePrivateKey() error = %v", err)
	}

	certPEM, keyPEM, err := generateCertificate(privateKey)
	if err != nil {
		t.Fatalf("generateCertificate() error = %v", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("generateCertificate() returned invalid certificate PEM")
	}
	if certBlock.Type != "CERTIFICATE" {
		t.Errorf("certificate PEM type = %s, want CERTIFICATE", certBlock.Type)
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	if cert.Subject.CommonName != "localhost" {
		t.Errorf("certificate CommonName = %s, want localhost", cert.Subject.CommonName)
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "Pullbase Development" {
		t.Errorf("certificate Organization = %v, want [Pullbase Development]", cert.Subject.Organization)
	}
	if len(cert.DNSNames) == 0 || cert.DNSNames[0] != "localhost" {
		t.Errorf("certificate DNSNames = %v, want [localhost]", cert.DNSNames)
	}
	if len(cert.IPAddresses) < 2 {
		t.Errorf("certificate IPAddresses = %v, want at least 2 IPs (127.0.0.1, ::1)", cert.IPAddresses)
	}

	now := time.Now()
	expectedExpiry := now.Add(365 * 24 * time.Hour)
	tolerance := 5 * time.Second

	if cert.NotBefore.After(now.Add(tolerance)) {
		t.Errorf("certificate NotBefore %v is too far in the future", cert.NotBefore)
	}
	if cert.NotAfter.Before(expectedExpiry.Add(-tolerance)) || cert.NotAfter.After(expectedExpiry.Add(tolerance)) {
		t.Errorf("certificate NotAfter = %v, want within %v of %v", cert.NotAfter, tolerance, expectedExpiry)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("generateCertificate() returned invalid key PEM")
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("key PEM type = %s, want EC PRIVATE KEY", keyBlock.Type)
	}

	_, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}
}

func TestGenerateCertificateInvalidKey(t *testing.T) {
	_, _, err := generateCertificate("not a key")
	if err == nil {
		t.Error("generateCertificate() with invalid key should return error")
	}
}

func TestGenerateSelfSignedCerts(t *testing.T) {
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "certs", "server.crt")
	keyPath := filepath.Join(tempDir, "certs", "server.key")

	err := generateSelfSignedCerts(certPath, keyPath)
	if err != nil {
		t.Fatalf("generateSelfSignedCerts() error = %v", err)
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("certificate file was not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("key file was not created")
	}

	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("failed to stat key file: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Errorf("key file permissions = %o, want 0600", keyInfo.Mode().Perm())
	}

	certInfo, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("failed to stat cert file: %v", err)
	}
	if certInfo.Mode().Perm() != 0o644 {
		t.Errorf("cert file permissions = %o, want 0644", certInfo.Mode().Perm())
	}

	_, err = tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("tls.LoadX509KeyPair() error = %v", err)
	}
}

func TestGenerateSelfSignedCertsDifferentDirectories(t *testing.T) {
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "certs", "server.crt")
	keyPath := filepath.Join(tempDir, "keys", "server.key")

	err := generateSelfSignedCerts(certPath, keyPath)
	if err != nil {
		t.Fatalf("generateSelfSignedCerts() with different directories error = %v", err)
	}

	if _, err := os.Stat(filepath.Dir(certPath)); os.IsNotExist(err) {
		t.Error("certificate directory was not created")
	}
	if _, err := os.Stat(filepath.Dir(keyPath)); os.IsNotExist(err) {
		t.Error("key directory was not created")
	}
}

func TestGenerateSelfSignedCertsCurrentDirectory(t *testing.T) {
	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	err = generateSelfSignedCerts("server.crt", "server.key")
	if err != nil {
		t.Fatalf("generateSelfSignedCerts() in current directory error = %v", err)
	}

	if _, err := os.Stat("server.crt"); os.IsNotExist(err) {
		t.Error("certificate file was not created in current directory")
	}
	if _, err := os.Stat("server.key"); os.IsNotExist(err) {
		t.Error("key file was not created in current directory")
	}
}

func TestGenerateBootstrapSecret(t *testing.T) {
	secret, err := generateBootstrapSecret()
	if err != nil {
		t.Fatalf("failed to generate bootstrap secret: %v", err)
	}
	if secret == "" {
		t.Fatal("generated bootstrap secret is empty")
	}
	_, err = base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		t.Errorf("failed to decode base64 encoded secret: %v", err)
	}
}

func TestWriteBootstrapSecretFile(t *testing.T) {
	tempDir := t.TempDir()
	secretPath := filepath.Join(tempDir, "bootstrap.secret")
	secret, err := generateBootstrapSecret()
	if err != nil {
		t.Fatalf("failed to generate bootstrap secret: %v", err)
	}
	err = writeBootstrapSecretFile(secretPath, secret)
	if err != nil {
		t.Fatalf("failed to write bootstrap secret file: %v", err)
	}
	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("failed to stat secret file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("bootstrap secret file permissions = %o, want 0600", info.Mode().Perm())
	}
	fileContent, err := readBootstrapSecretFile(secretPath)
	if err != nil {
		t.Fatalf("failed to read bootstrap secret file: %v", err)
	}
	if fileContent != secret {
		t.Fatalf("expected file content to be: %s, got: %s", secret, fileContent)
	}
}
func TestWriteBootstrapSecretFileDifferentDirectory(t *testing.T) {
	tempDir := t.TempDir()
	secretPath := filepath.Join(tempDir, "secrets", "bootstrap.secret")
	secret, err := generateBootstrapSecret()
	if err != nil {
		t.Fatalf("failed to generate bootstrap secret: %v", err)
	}
	err = writeBootstrapSecretFile(secretPath, secret)
	if err != nil {
		t.Fatalf("failed to write bootstrap secret file: %v", err)
	}
	dirInfo, err := os.Stat(filepath.Dir(secretPath))
	if err != nil {
		t.Fatalf("failed to stat bootstrap secret directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("bootstrap secret directory permissions = %o, want 0700", dirInfo.Mode().Perm())
	}
}

func TestWriteBootstrapSecretFileEmptyPath(t *testing.T) {
	err := writeBootstrapSecretFile("", "secret")
	if err == nil {
		t.Error("expected error when path is empty, got none")
	}
}

func TestReadBootstrapSecretFile(t *testing.T) {
	tests := []struct{
		name string
		pathFunc func(t *testing.T) (path, secret string)
		wantError bool
		errSubString string
	}{
		{
			name: "path is invalid",
			pathFunc: func(t *testing.T) (string, string) { return "path/does-not-exist", "" },
			wantError: true,
			errSubString: "failed to stat path",
		},
		{
			name: "path is directory",
			pathFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				return tempDir, ""
			},
			wantError: true,
			errSubString: "is a directory",
		},
		{
			name: "file permissions are invalid",
			pathFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				secretPath := filepath.Join(tempDir, "bootstrap.secret")
				file, err := os.Create(secretPath)
				if err != nil {
					t.Fatalf("failed to create secret file: %v", err)
				}
				if err = file.Close(); err != nil {
					t.Fatal("failed to close secret file")
				}
				return secretPath, ""
			},
			wantError: true,
			errSubString: "must not be group- or world-accessible",
		},
		{
			name: "valid secret file",
			pathFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				secretPath := filepath.Join(tempDir, "bootstrap.secret")
				secret, err := generateBootstrapSecret()
				if err != nil {
					t.Fatalf("failed to generate bootstrap secret: %v", err)
				}
				err = writeBootstrapSecretFile(secretPath, secret)
				if err != nil {
					t.Fatalf("failed to write bootstrap secret file: %v", err)
				}
				return secretPath, secret
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			secretPath, secret := tt.pathFunc(t)
			readSecret, err := readBootstrapSecretFile(secretPath)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error when %s, got none", tt.name)
				}
				if !strings.Contains(err.Error(), tt.errSubString) {
					t.Fatalf("expected error to contain %q, got: %v", tt.errSubString, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error when reading secret: %v", err)
			}
			if readSecret == "" {
				t.Fatalf("expected to read a secret, got empty string")
			}
			if secret != readSecret {
				t.Fatalf("expected read secret to be: %s, got: %s", secret, readSecret)
			}
		})
	}
}
