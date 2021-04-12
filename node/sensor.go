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

func (n *Node) loadSensor(ctx context.Context, cfg *config.Sensor) error {
	log.Printf("loading sensor %s", cfg.Platform)
	switch cfg.Platform {
	case "bme280":
		if err := n.loadSensorBMxx80(ctx, cfg); err != nil {
			return fmt.Errorf("sensor(%s): %w", cfg.Platform, err)
		}
		return nil
	case "fake":
		if err := n.loadSensorFake(ctx, cfg); err != nil {
			return fmt.Errorf("sensor(%s): %w", cfg.Platform, err)
		}
		return nil
	case "wifi_signal":
		if err := n.loadSensorWifiSignal(ctx, cfg); err != nil {
			return fmt.Errorf("sensor(%s): %w", cfg.Platform, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown platform %q", cfg.Platform)
	}
}
