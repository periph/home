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

func (n *Node) loadBinarySensor(ctx context.Context, cfg *config.BinarySensor) error {
	log.Printf("loading binary_sensor %s", cfg.Platform)
	switch cfg.Platform {
	case "fake":
		if err := n.loadBinarySensorFake(ctx, cfg); err != nil {
			return fmt.Errorf("binary_sensor(%s): %w", cfg.Name, err)
		}
		return nil
	case "gpio":
		if err := n.loadBinarySensorGPIO(ctx, cfg); err != nil {
			return fmt.Errorf("binary_sensor(%s): %w", cfg.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown platform %q", cfg.Platform)
	}
}
