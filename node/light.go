// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"fmt"
	"log"

	"periph.io/x/home/node/config"
)

func (n *Node) loadLight(ctx context.Context, cfg *config.Light) error {
	log.Printf("loading light %s", cfg.Platform)
	switch cfg.Platform {
	case "apa102":
		if err := n.loadLightAPA102(ctx, cfg); err != nil {
			return fmt.Errorf("light(%s): %w", cfg.Name, err)
		}
		return nil
	case "fake":
		if err := n.loadLightFake(ctx, cfg); err != nil {
			return fmt.Errorf("light(%s): %w", cfg.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown platform %q", cfg.Platform)
	}
}
