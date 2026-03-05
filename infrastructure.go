// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2019-2026 The Namecoin developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package minincrpcclient

import (
	"context"
	"encoding/base64"
	"os"
	"time"

	"github.com/ybbus/jsonrpc/v3"
)

// Client represents a Namecoin RPC client which allows easy access to the
// various RPC methods available on a Namecoin RPC server.  Each of the wrapper
// functions handle the details of converting the passed and return types to and
// from the underlying JSON types which are required for the JSON-RPC
// invocations
type Client struct {
	c jsonrpc.RPCClient

	config *ConnConfig
}

// TODO: ask ybbus about making auth mutable, would simplify this a lot
func (client *Client) reset() error {
	endpoint := "http://" + client.config.Host
	user, pass, err := client.config.getAuth()
	if err != nil {
		client.c = nil
		return err
	}

	rpcClient := jsonrpc.NewClientWithOpts(endpoint, &jsonrpc.RPCClientOpts{
		AllowUnknownFields: true,
		CustomHeaders: map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
		},
	})

	client.c = rpcClient

	return nil
}

func (client *Client) CallFor(ctx context.Context, out interface{}, method string, params ...interface{}) error {
	err := client.c.CallFor(ctx, out, method, params...)

	// Detect an auth failure
	if err != nil {
		herr, ok := err.(*jsonrpc.HTTPError)
		if ok {
			if herr.Code == 401 {
				// Try refreshing the auth
				err = client.reset()
				if err != nil {
					return err
				}

				err = client.c.CallFor(ctx, out, method, params...)
			}
		}
	}

	return err
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
	client := &Client{c: nil, config: config}

	err := client.reset()

	return client, err
}
