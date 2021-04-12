// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config_test

import (
	"fmt"
	"log"

	"periph.io/x/home/node/config"
)

// See https://github.com/periph/home/blob/main/example.yaml
// for a full example of a periphhome configuration file.
const sampleConf = `
periphhome:
  name: pi
  comment: pi device

api:

binary_sensor:
# ...

sensor:
# ...
`

func Example() {
	cfg := config.Root{}
	if err := cfg.LoadYaml([]byte(sampleConf)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Device: %s\n", cfg.PeriphHome.Name)

	// Output:
	// Device: pi
}
