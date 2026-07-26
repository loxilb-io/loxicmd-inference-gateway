/*
 * Copyright (c) 2025 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSelfSignedCert generates a self-signed cert/key pair and writes them as
// PEM files under dir, returning their paths.
func writeSelfSignedCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "loxicmd-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certPEM, _ := os.Create(certPath)
	defer certPEM.Close()
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode cert: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM, _ := os.Create(keyPath)
	defer keyPEM.Close()
	if err := pem.Encode(keyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	return certPath, keyPath
}

func TestNeedsTLSConfig(t *testing.T) {
	tests := []struct {
		name string
		o    RESTOptions
		want bool
	}{
		{"plain http, no opts", RESTOptions{Protocol: "http"}, false},
		{"https", RESTOptions{Protocol: "https"}, true},
		{"http with insecure", RESTOptions{Protocol: "http", Insecure: true}, true},
		{"http with cacert", RESTOptions{Protocol: "http", CACertFile: "/x"}, true},
		{"http with client cert", RESTOptions{Protocol: "http", ClientCertFile: "/x"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsTLSConfig(&tc.o); got != tc.want {
				t.Fatalf("needsTLSConfig = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildTLSConfig_Insecure(t *testing.T) {
	cfg, err := buildTLSConfig(&RESTOptions{Protocol: "https", Insecure: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = false, want true")
	}
}

func TestBuildTLSConfig_CACertAndClientCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir)

	cfg, err := buildTLSConfig(&RESTOptions{
		Protocol:       "https",
		CACertFile:     certPath,
		ClientCertFile: certPath,
		ClientKeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs = nil, want a populated pool")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

func TestBuildTLSConfig_Errors(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir)

	bogus := filepath.Join(dir, "bogus.pem")
	if err := os.WriteFile(bogus, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		o    RESTOptions
	}{
		{"missing CA file", RESTOptions{CACertFile: filepath.Join(dir, "nope.pem")}},
		{"invalid CA content", RESTOptions{CACertFile: bogus}},
		{"cert without key", RESTOptions{ClientCertFile: certPath}},
		{"key without cert", RESTOptions{ClientKeyFile: keyPath}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildTLSConfig(&tc.o); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}
