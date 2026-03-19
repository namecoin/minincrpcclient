// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2019-2026 The Namecoin developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package minincrpcclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      uint64      `json:"id"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
// Result is kept as json.RawMessage to avoid an intermediate interface{}
// parse and the resulting re-marshal round-trip that was the primary
// bottleneck in the ybbus/jsonrpc library.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      uint64          `json:"id"`
}

// jsonRPCError represents a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// HTTPError represents an error that occurred at the HTTP transport level.
type HTTPError struct {
	Code int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error: status code %d", e.Code)
}

// Client represents a Namecoin RPC client which allows easy access to the
// various RPC methods available on a Namecoin RPC server.  Each of the wrapper
// functions handle the details of converting the passed and return types to and
// from the underlying JSON types which are required for the JSON-RPC
// invocations
type Client struct {
	httpClient *http.Client
	endpoint   string

	config *ConnConfig

	nextID uint64
}

// authHeader returns the current HTTP Basic Authorization header value.
// Because we read credentials via ConnConfig.getAuth() on every call,
// cookie-based credentials are always up to date — there is no stale
// cached header.  This is the "mutable auth" that ybbus/jsonrpc could
// not provide (see issue #1).
func (client *Client) authHeader() (string, error) {
	user, pass, err := client.config.getAuth()
	if err != nil {
		return "", err
	}

	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)), nil
}

func (client *Client) CallFor(ctx context.Context, out interface{}, method string, params ...interface{}) error {
	id := atomic.AddUint64(&client.nextID, 1)

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Fetch credentials fresh for every request.  ConnConfig.getAuth()
	// uses a 30-second TTL cache for cookie files, so this is cheap for
	// the common case while still picking up rotated cookies promptly.
	auth, err := client.authHeader()
	if err != nil {
		return fmt.Errorf("get auth: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", client.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", auth)

	httpResp, err := client.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("rpc call %s: %w", method, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		io.Copy(io.Discard, httpResp.Body) //nolint:errcheck

		return &HTTPError{Code: httpResp.StatusCode}
	}

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var rpcResp jsonRPCResponse

	err = json.Unmarshal(respBytes, &rpcResp)
	if err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	// Unmarshal the result directly from raw JSON bytes into the target
	// type.  This is the key performance improvement: we skip the
	// intermediate interface{} representation that ybbus/jsonrpc used,
	// which required a re-marshal round-trip (interface{} -> []byte ->
	// target struct).
	if out != nil && rpcResp.Result != nil {
		err = json.Unmarshal(rpcResp.Result, out)
		if err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// Adapted from btcd
type ConnConfig struct {
	// Host is the IP address and port of the RPC server you want to connect
	// to.
	Host string

	// User is the username to use to authenticate to the RPC server.
	User string

	// Pass is the passphrase to use to authenticate to the RPC server.
	Pass string

	// CookiePath is the path to a cookie file containing the username and
	// passphrase to use to authenticate to the RPC server.  It is used
	// instead of User and Pass if non-empty.
	CookiePath string

	cookieLastCheckTime time.Time
	cookieLastModTime   time.Time
	cookieLastUser      string
	cookieLastPass      string
	cookieLastErr       error

	// If you need other btcd config options, please file an issue.
}

// getAuth returns the username and passphrase that will actually be used for
// this connection.  This will be the result of checking the cookie if a cookie
// path is configured; if not, it will be the user-configured username and
// passphrase.
func (config *ConnConfig) getAuth() (username, passphrase string, err error) {
	// Try username+passphrase auth first.
	if config.Pass != "" {
		return config.User, config.Pass, nil
	}

	// If no username or passphrase is set, try cookie auth.
	return config.retrieveCookie()
}

// retrieveCookie returns the cookie username and passphrase.
func (config *ConnConfig) retrieveCookie() (username, passphrase string, err error) {
	if !config.cookieLastCheckTime.IsZero() && time.Now().Before(config.cookieLastCheckTime.Add(30*time.Second)) {
		return config.cookieLastUser, config.cookieLastPass, config.cookieLastErr
	}

	config.cookieLastCheckTime = time.Now()

	st, err := os.Stat(config.CookiePath)
	if err != nil {
		config.cookieLastErr = err
		return config.cookieLastUser, config.cookieLastPass, config.cookieLastErr
	}

	modTime := st.ModTime()
	if !modTime.Equal(config.cookieLastModTime) {
		config.cookieLastModTime = modTime
		config.cookieLastUser, config.cookieLastPass, config.cookieLastErr = readCookieFile(config.CookiePath)
	}

	return config.cookieLastUser, config.cookieLastPass, config.cookieLastErr
}

// New creates a new RPC client based on the provided connection configuration
// details.  The notification handlers parameter may be nil if you are not
// interested in receiving notifications and will be ignored if the
// configuration is set to run in HTTP POST mode.
func New(config *ConnConfig) (*Client, error) {
	// Verify that credentials are accessible at creation time.
	_, _, err := config.getAuth()
	if err != nil {
		return nil, fmt.Errorf("get initial auth: %w", err)
	}

	client := &Client{
		httpClient: &http.Client{},
		endpoint:   "http://" + config.Host,
		config:     config,
	}

	return client, nil
}
