/*
 * Copyright (c) 2022 NetLOX Inc
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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	DEFAULT_URL_FORMAT = "%s://%s:%d"
)

type RequestInfo struct {
	provider    string
	apiVersion  string
	resource    string
	subResource []string
	queryArgs   map[string]string
}

func (r *RequestInfo) makeBaseURL() string {
	p := path.Join(r.provider, r.apiVersion)
	if len(r.resource) != 0 {
		p = path.Join(p, r.resource)
	}

	if len(r.subResource) != 0 {
		subP := path.Join(r.subResource...)
		p = path.Join(p, subP)
	}
	return p
}

// GetBaseURL return url.URL.Path string
func (r *RequestInfo) GetBaseURL() string {
	return r.makeBaseURL()
}

// GetQueryValue return url.Values for url.URL
func (r *RequestInfo) GetQueryString() string {
	return url.Values{}.Encode()
}

type RESTOptions struct {
	PrintOption string
	Protocol    string
	ServerIP    string
	ServerPort  int16
	Timeout     int16
	ServiceName string
	Token       string
	// BearerAuth, when true, prefixes the Authorization header value with
	// "Bearer ". The loxilb-inference-gateway JWT middleware expects this
	// form; classic loxilb targets that accept a raw token can disable it.
	BearerAuth bool
	// TLS options (used when Protocol == "https").
	Insecure       bool   // skip server certificate verification
	CACertFile     string // PEM CA bundle to verify the server certificate
	ClientCertFile string // client certificate for mTLS
	ClientKeyFile  string // client private key for mTLS
}

type RESTClient struct {
	Options RESTOptions
	Client  *http.Client
}

func (r *RESTClient) GetProcotol() string {
	return r.Options.Protocol
}

func (r *RESTClient) GetHost() string {
	return fmt.Sprintf("%s:%d", r.Options.ServerIP, int(r.Options.ServerPort))
}

func (r *RESTClient) GET(ctx context.Context, getURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

func (r *RESTClient) POST(ctx context.Context, postURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

func (r *RESTClient) DELETE(ctx context.Context, deleteURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

// DELETEWithBody issues a DELETE carrying a JSON request body (used by
// endpoints such as /sni/certificates that identify the target in the body).
func (r *RESTClient) DELETEWithBody(ctx context.Context, deleteURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

func (r *RESTClient) PATCH(ctx context.Context, patchURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

// PUT issues an HTTP PUT with a JSON request body (used by endpoints such as
// /config/l4trace/sampling that replace a resource wholesale).
func (r *RESTClient) PUT(ctx context.Context, putURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	r.getTokens()
	req.Header.Set("Content-Type", "application/json")
	r.setAuthHeader(req)
	return r.Client.Do(req)
}

// setAuthHeader sets the Authorization header from the configured token.
// When BearerAuth is enabled the value is prefixed with "Bearer " (unless it
// already carries that prefix), matching what the inference gateway expects.
// No header is set when there is no token, so unauthenticated targets are
// unaffected.
func (r *RESTClient) setAuthHeader(req *http.Request) {
	token := strings.TrimSpace(r.Options.Token)
	if token == "" {
		return
	}
	if r.Options.BearerAuth && !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
}

func (r *RESTClient) getTokens() {
	if r.Options.Token == "" {
		token, err := os.ReadFile("/tmp/loxilbtoken")
		if err != nil {
			return
		}
		r.Options.Token = strings.TrimSpace(string(token))
	}
}
