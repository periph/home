// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package config contains all the structures used to represent the YAML file
// to load a periph-home node.
//
// The file schema starts with the type Root.
//
// Configuration
//
// The configuration yaml file is expected to look like this:
//
//   periphhome:
//
//   api:
//
//   binary_sensor:
//     - platform: gpio
//       name: "Motion sensor"
//       device_class: motion
//       pin:
//         number: GPIO17
//         mode: INPUT_PULLUP
//
//   sensor:
//     - platform: bme280
//       address: 0x76
//       update_interval: 60s
//       temperature:
//         name: "Temperature"
//       pressure:
//         name: "Pressure"
//       humidity:
//         name: "Humidity"
//     - platform: wifi_signal
//       name: "wifi signal"
//
package config

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

// PinMode is the value for "pin/mode".
type PinMode string

// validate validates the configuration.
func (p PinMode) validate() error {
	switch p {
	case "",
		Input,
		Output,
		OutputOpenDrain,
		Analog,
		InputPullup,
		InputPulldown:
		return nil
	default:
		return errors.New("invalid pin mode")
	}
}

// Valid PinMode values.
const (
	Input           PinMode = "INPUT"
	InputPullup     PinMode = "INPUT_PULLUP"
	InputPulldown   PinMode = "INPUT_PULLDOWN"
	Analog          PinMode = "ANALOG"
	Output          PinMode = "OUTPUT"
	OutputOpenDrain PinMode = "OUTPUT_OPEN_DRAIN"
)

// Root is the configuration file format.
//
// It is designed to look like ESPHome configuration yaml but has differences
// where appropriate.
type Root struct {
	PeriphHome    PeriphHome     `yaml:"periphhome"`
	API           API            `yaml:"api"`
	BinarySensors []BinarySensor `yaml:"binary_sensor"`
	Sensors       []Sensor       `yaml:"sensor"`
	Lights        []Light        `yaml:"light"`
	Cameras       []Camera       `yaml:"camera"`

	_ struct{}
}

// LoadYaml loads the config from serialized yaml.
//
// It is a utility function that deserialize the yaml with strict checking. It
// also performs validation.
//
// The validation is not strict, it could still fail loading when passed to
// node.New().
func (r *Root) LoadYaml(b []byte) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	// Save the user trouble when they are doing a typo.
	d.SetStrict(true)
	if err := d.Decode(r); err != nil {
		return err
	}
	return r.validate()
}

// validate validates the configuration.
func (r *Root) validate() error {
	if err := r.PeriphHome.validate(); err != nil {
		return err
	}
	if err := r.API.validate(); err != nil {
		return err
	}
	for i := range r.BinarySensors {
		if err := r.BinarySensors[i].validate(); err != nil {
			return err
		}
	}
	for i := range r.Sensors {
		if err := r.Sensors[i].validate(); err != nil {
			return err
		}
	}
	for i := range r.Lights {
		if err := r.Lights[i].validate(); err != nil {
			return err
		}
	}
	if len(r.Cameras) > 1 {
		return errors.New("the ESPHome protocol currently only support one camera per node; please contribute upstream to add support for multiple cameras")
	}
	for i := range r.Cameras {
		if err := r.Cameras[i].validate(); err != nil {
			return err
		}
	}
	return nil
}

// PeriphHome is the "periphhome" section.
type PeriphHome struct {
	// Name is the name that will be shown in Home Assistant.
	// Defaults to the hostname.
	Name    string
	Comment string

	_ struct{}
}

// validate validates the configuration.
func (p *PeriphHome) validate() error {
	if len(p.Name) > 63 {
		return errors.New("periphhome: name is too long")
	}
	return nil
}

// API is the "api" section.
type API struct {
	// Port is the TCP port for the native API.
	//
	// Defaults to 6053.
	Port int
	// Password provides a very weak protection, since no encryption and no
	// hashing is used.
	Password string

	// IsPresent is set to true if the field was present when the configuration
	// is deserialized from yaml.
	IsPresent bool `yaml:"-"`
	_         struct{}
}

type api struct {
	Port     int
	Password string
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (a *API) UnmarshalYAML(unmarshal func(interface{}) error) error {
	t := api{}
	if err := unmarshal(&t); err != nil {
		return err
	}
	a.Port = t.Port
	a.Password = t.Password
	a.IsPresent = true
	return nil
}

// validate validates the configuration.
func (a *API) validate() error {
	if a.Port < 0 || a.Port >= 65536 {
		return errors.New("api: port is invalid")
	}
	return nil
}

// BinarySensor is an element in the "binary_sensor" section.
type BinarySensor struct {
	Platform    string
	Name        string
	DeviceClass string `yaml:"device_class"`
	Pin         Pin

	_ struct{}
}

// validate validates the configuration.
func (b *BinarySensor) validate() error {
	if b.Name == "" {
		return errors.New("binary_sensor: name is required")
	}
	return b.Pin.validate()
}

// Pin is a "pin" section.
type Pin struct {
	Number   string
	Inverted bool
	Mode     PinMode

	_ struct{}
}

// validate validates the configuration.
func (p *Pin) validate() error {
	return p.Mode.validate()
}

// Sensor is an element in the "sensor" section.
type Sensor struct {
	Platform       string
	Name           string
	Temperature    SensorParams
	Pressure       SensorParams
	Humidity       SensorParams
	Address        int
	UpdateInterval time.Duration `yaml:"update_interval"`

	_ struct{}
}

// validate validates the configuration.
func (s *Sensor) validate() error {
	if s.Platform == "" {
		return errors.New("sensor: platform is required")
	}
	if err := s.Temperature.validate(); err != nil {
		return fmt.Errorf("sensor / temperature: %w", err)
	}
	if err := s.Pressure.validate(); err != nil {
		return fmt.Errorf("sensor / pressure: %w", err)
	}
	if err := s.Humidity.validate(); err != nil {
		return fmt.Errorf("sensor / humidity: %w", err)
	}
	return nil
}

// SensorParams defines a sensor parameter.
type SensorParams struct {
	Name string

	_ struct{}
}

// validate validates the configuration.
func (s *SensorParams) validate() error {
	return nil
}

// Light is an element in the "light" section.
type Light struct {
	Platform string
	Name     string
	NumLEDs  int `yaml:"num_leds"`

	_ struct{}
}

// validate validates the configuration.
func (l *Light) validate() error {
	if l.Platform == "" {
		return errors.New("light: platform is required")
	}
	if l.Name == "" {
		return errors.New("light: name is required")
	}
	if l.NumLEDs < 0 || l.NumLEDs > 1000000 {
		return errors.New("light: num_leds is required")
	}
	return nil
}

// Camera is an element in the "camera" section.
type Camera struct {
	Platform  string
	Name      string
	Directory string
	Rotation  int

	_ struct{}
}

// validate validates the configuration.
func (c *Camera) validate() error {
	if c.Platform == "" {
		return errors.New("camera: platform is required")
	}
	if c.Name == "" {
		return errors.New("camera: name is required")
	}
	if c.Directory != "" {
		if !filepath.IsAbs(c.Directory) {
			// Save the user trouble since when started via systemd the working
			// directory will not match.
			return errors.New("camera: directory must be absolute path")
		}
	}
	switch c.Rotation {
	case 0, 90, 180, 270:
	default:
		return errors.New("camera: invalid rotation")
	}
	return nil
}
