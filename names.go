// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2019-2026 The Namecoin developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package minincrpcclient

import (
	"context"
	"encoding/hex"
	"fmt"

	ncbtcjson "github.com/namecoin/minincbtcjson"
)

// *********************
// Name Lookup Functions
// *********************

// NameShow returns detailed information about a name.
func (c *Client) NameShow(name string, options *ncbtcjson.NameShowOptions) (*ncbtcjson.NameShowResult, error) {
	if options == nil {
		options = &ncbtcjson.NameShowOptions{}
	}

	options.NameEncoding, options.ValueEncoding = ncbtcjson.Hex, ncbtcjson.Hex
	name = hex.EncodeToString([]byte(name))

	var nameShow *ncbtcjson.NameShowResult

	err := c.CallFor(context.Background(), &nameShow, "name_show", name, options)

	if err != nil {
		return nil, err
	}

	if nameShow == nil {
		return nil, nil
	}

	if nameShow.NameEncoding == ncbtcjson.Hex {
		var nameBytes []byte

		nameBytes, err = hex.DecodeString(nameShow.Name)
		if err != nil {
			return nil, fmt.Errorf("decode hex name: %w", err)
		}

		nameShow.Name = string(nameBytes)
	}

	if nameShow.ValueEncoding == ncbtcjson.Hex {
		var valueBytes []byte

		valueBytes, err = hex.DecodeString(nameShow.Value)
		if err != nil {
			return nil, fmt.Errorf("decode hex value: %w", err)
		}

		nameShow.Value = string(valueBytes)
	}

	return nameShow, nil
}

// NameScan returns detailed information about a list of names.
// TODO: handle options and hex encoding
func (c *Client) NameScan(start string, count uint32) (ncbtcjson.NameScanResult, error) {
	var nameScan *ncbtcjson.NameScanResult

	err := c.CallFor(context.Background(), &nameScan, "name_scan", start, count)

	if err != nil {
		return nil, err
	}

	if nameScan == nil {
		return nil, nil
	}

	return *nameScan, nil
}
