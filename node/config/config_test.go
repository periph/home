// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const sampleConf = `
periphhome:
  name: pi
  comment: pi device

api:
  port: 6053
  password: "Foo"

binary_sensor:
  - platform: gpio
    name: "Motion sensor"
    device_class: motion
    pin:
      number: GPIO17
      inverted: true
      mode: INPUT_PULLUP

sensor:
  - platform: bme280
    address: 0x76
    update_interval: 60s
    temperature:
      name: "Temperature"
    pressure:
      name: "Pressure"
    humidity:
      name: "Humidity"
  - platform: wifi_signal
    name: "Foo Wifi Signal"
    update_interval: 60s

light:
  - platform: apa102
    name: "Bright lights"
    num_leds: 150

camera:
  - platform: fake
    name: "Fake Camera"
`

func TestRootLoadYaml(t *testing.T) {
	got := Root{}
	if err := got.LoadYaml([]byte(sampleConf)); err != nil {
		t.Fatal(err)
	}
	want := Root{
		PeriphHome: PeriphHome{
			Name:    "pi",
			Comment: "pi device",
		},
		API: API{
			Port:      6053,
			IsPresent: true,
			Password:  "Foo",
		},
		BinarySensors: []BinarySensor{
			{
				Platform:    "gpio",
				Name:        "Motion sensor",
				DeviceClass: "motion",
				Pin: Pin{
					Number:   "GPIO17",
					Inverted: true,
					Mode:     "INPUT_PULLUP",
				},
			},
		},
		Sensors: []Sensor{
			{
				Platform:       "bme280",
				Temperature:    SensorParams{Name: "Temperature"},
				Pressure:       SensorParams{Name: "Pressure"},
				Humidity:       SensorParams{Name: "Humidity"},
				Address:        0x76,
				UpdateInterval: time.Minute,
			},
			{
				Platform:       "wifi_signal",
				Name:           "Foo Wifi Signal",
				UpdateInterval: time.Minute,
			},
		},
		Lights: []Light{
			{
				Platform: "apa102",
				Name:     "Bright lights",
				NumLEDs:  150,
			},
		},
		Cameras: []Camera{
			{
				Platform: "fake",
				Name:     "Fake Camera",
			},
		},
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(API{})); diff != "" {
		t.Errorf("Root mismatch (-want +got):\n%s", diff)
	}
}

func TestRootLoadYaml_Err(t *testing.T) {
	got := Root{}
	if err := got.LoadYaml([]byte("unexpected: false")); err == nil {
		t.Fatal("expected error")
	} else if diff := cmp.Diff("yaml: unmarshal errors:\n  line 1: field unexpected not found in type config.Root", err.Error()); diff != "" {
		// Crappy test, comment out if yaml changes its error message.
		t.Fatal(diff)
	}
}

func TestRootLoadYaml_Minimal(t *testing.T) {
	// A minimally viable configuration: an empty file.
	got := Root{}
	if err := got.LoadYaml([]byte("periphhome:\n")); err != nil {
		t.Fatal(err)
	}
	want := Root{}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(API{})); diff != "" {
		t.Errorf("Root mismatch (-want +got):\n%s", diff)
	}
	if got.API.IsPresent {
		t.FailNow()
	}
}

/*
func TestRootLoadYaml_ExampleFile(t *testing.T) {
	got := Root{}
	if err := got.LoadYaml(filepath.Join("..", "..", "example.yaml")); err != nil {
		t.Fatal(err)
	}
	want := Root{
		PeriphHome: PeriphHome{
			Name:    "pi",
			Comment: "pi device",
		},
		API: API{
			Port:     6053,
			Password: "Foo",
		},
		BinarySensors: []BinarySensor{
			{
				Platform:    "gpio",
				Name:        "Motion sensor",
				DeviceClass: "motion",
				Pin: Pin{
					Number:   "GPIO17",
					Inverted: true,
					Mode:     "INPUT_PULLUP",
				},
			},
		},
		Sensors: []Sensor{
			{
				Platform:       "bme280",
				Temperature:    SensorParams{Name: "Temperature"},
				Pressure:       SensorParams{Name: "Pressure"},
				Humidity:       SensorParams{Name: "Humidity"},
				Address:        0x76,
				UpdateInterval: time.Minute,
			},
			{
				Platform:       "wifi_signal",
				Name:           "Foo Wifi Signal",
				UpdateInterval: time.Minute,
			},
		},
		Lights: []Light{
			{
				Platform: "apa102",
				Name:     "Bright lights",
				NumLEDs:  150,
			},
		},
		Cameras: []Camera{
			{
				Platform: "raspistill",
				Name:     "RPi Camera",
			},
		},
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(API{})); diff != "" {
		t.Errorf("Root mismatch (-want +got):\n%s", diff)
	}
}
*/
