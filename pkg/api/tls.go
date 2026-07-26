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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// needsTLSConfig reports whether a custom TLS configuration is required for the
// given options. A default transport is fine unless the caller talks HTTPS or
// supplies any TLS-related option.
func needsTLSConfig(o *RESTOptions) bool {
	return o.Protocol == "https" || o.Insecure ||
		o.CACertFile != "" || o.ClientCertFile != "" || o.ClientKeyFile != ""
}

// buildTLSConfig assembles a *tls.Config from the REST options: optional server
// verification skip, a custom CA bundle, and a client certificate for mTLS.
func buildTLSConfig(o *RESTOptions) (*tls.Config, error) {
	cfg := &tls.Config{
		InsecureSkipVerify: o.Insecure, //nolint:gosec // opt-in via --insecure
	}

	if o.CACertFile != "" {
		pem, err := os.ReadFile(o.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert %q: %w", o.CACertFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no valid certificates found in CA cert %q", o.CACertFile)
		}
		cfg.RootCAs = pool
	}

	if o.ClientCertFile != "" || o.ClientKeyFile != "" {
		if o.ClientCertFile == "" || o.ClientKeyFile == "" {
			return nil, fmt.Errorf("--cert and --key must be supplied together")
		}
		cert, err := tls.LoadX509KeyPair(o.ClientCertFile, o.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading client cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	return cfg, nil
}
