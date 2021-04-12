// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// periphhome-client is a client implementation of the ESPHome protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"periph.io/x/home/client"
)

func mainImpl() error {
	wait := flag.Duration("wait", time.Second/2, "Time to wait for discovery, increase if not all devices are found")
	first := flag.Bool("first", false, "Stop waiting after the first device found")
	flag.Parse()

	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	found, err := client.Search(ctx, *first)
	if err != nil {
		return err
	}
	fmt.Printf("Found %d device(s)\n", len(found))
	for _, d := range found {
		fmt.Printf("- %s\n", d)
	}
	return nil
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "periphhome-client: %s\n", err)
		os.Exit(1)
	}
}
