// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2019-2026 The Namecoin developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"log"

	ncrpcclient "github.com/namecoin/minincrpcclient"
)

func main() {
	// Connect to local namecoin core RPC server using HTTP POST mode.
	connCfg := &ncrpcclient.ConnConfig{
		Host:         "localhost:8336",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := ncrpcclient.New(connCfg)
	if err != nil {
		log.Fatal(err)
	}

	// Get the current data for the name.
	nameData, err := client.NameShow("d/domob", nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Name: %s", nameData.Name)
	log.Printf("Value: %s", nameData.Value)
}
