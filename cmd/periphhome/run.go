// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"log"

	"periph.io/x/home/node"
	"periph.io/x/home/node/config"
)

func run(ctx context.Context, cfg *config.Root) error {
	// TODO(maruel): When running as a service, the lines are already annotated,
	// so no need to set the timestamp.
	//log.SetFlags(0)

	n, err := node.New(ctx, cfg)
	if err != nil {
		return err
	}
	log.Printf("node initialized")
	<-ctx.Done()
	log.Printf("closing node")
	return n.Close()
}
