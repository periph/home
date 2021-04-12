// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"html/template"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/gpio/gpiotest"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/conn/v3/spi/spitest"
	"periph.io/x/home/node/config"
)

const sampleConf = `
periphhome:
  name: pi
  comment: pi device

api:
  port: 6053
  password: "Foo"

binary_sensor:
  - platform: fake
    name: "fake binary_sensor"
    device_class: motion

camera:
  - platform: fake
    name: "fake camera"

light:
  - platform: fake
    name: "fake light"

sensor:
  - platform: fake
    name: "fake sensor"
    update_interval: 60s
`

var wantPython = template.Must(template.New("").Parse(`API version: APIVersion(major=1, minor=3)
Device info: DeviceInfo(uses_password=True, name='pi', mac_address='{{.Mac}}', compilation_time='pi device', model='{{.GOOS}}', has_deep_sleep=False, esphome_version='PeriphHome {{.Version}}')

Entities:
- BinarySensorInfo(object_id='fakebinary_sensor', key=2604849794, name='fake binary_sensor', unique_id='pibinary_sensorfakebinary_sensor', device_class='motion', is_status_binary_sensor=False)
- SensorInfo(object_id='fakesensor', key=3490831464, name='fake sensor', unique_id='pisensorfakesensor', icon='mdi:exclamation', device_class='', unit_of_measurement='', accuracy_decimals=0, force_update=False)
- LightInfo(object_id='fakelight', key=2124765894, name='fake light', unique_id='pilightfakelight', supports_brightness=False, supports_rgb=False, supports_white_value=False, supports_color_temperature=False, min_mireds=0.0, max_mireds=0.0, effects=[])
- CameraInfo(object_id='fakecamera', key=1841563375, name='fake camera', unique_id='picamerafakecamera')

State:
- CameraState(key=1841563375, image=b'<elided>')
- LightState(key=2124765894, state=False, brightness=0.0, red=0.0, green=0.0, blue=0.0, white=0.0, color_temperature=0.0, effect='')
- BinarySensorState(key=2604849794, state=False, missing_state=False)
- SensorState(key=3490831464, state=1.0, missing_state=False)
`))

var debug = flag.Bool("debug", false, "debug python output")

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that fetches python virtualenv in -short mode")
	}
	shouldLog = testing.Verbose()

	// Register fake devices.
	p := gpiotest.Pin{
		N:         "FAKE_GPIO0",
		EdgesChan: make(chan gpio.Level),
	}
	if err := gpioreg.Register(&p); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gpioreg.Unregister("FAKE_GPIO0"); err != nil {
			t.Error(err)
		}
	}()
	o := func() (spi.PortCloser, error) {
		return &spitest.Record{}, nil
	}
	if err := spireg.Register("FAKE_SPI", nil, 0, o); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := spireg.Unregister("FAKE_SPI"); err != nil {
			t.Error(err)
		}
	}()

	cfg := config.Root{}
	if err := cfg.LoadYaml([]byte(sampleConf)); err != nil {
		t.Fatal(err)
	}
	// Find a free port.
	cfg.API.Port = getFreePort(t)
	n, err := New(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = n.Close(); err != nil {
			t.Error(err)
		}
	}()
	python := filepath.Join("testdata", "venv", "bin", "python")
	if runtime.GOOS == "windows" {
		python = filepath.Join("testdata", "venv", "Scripts", "python.exe")
	}
	if _, err = os.Stat(python); errors.Is(err, os.ErrNotExist) {
		s := filepath.Join("testdata", "setup")
		if runtime.GOOS == "windows" {
			s += ".bat"
		} else {
			s += ".sh"
		}
		t.Logf("installing virtualenv")
		if out, err2 := exec.Command(s).CombinedOutput(); err2 != nil {
			t.Fatal(string(out))
		}
	}
	// For debugging. Don't key on testing.Verbose() since the test would be
	// failing.
	if *debug {
		cmd := exec.Command(python, filepath.Join("testdata", "test.py"), "--port", strconv.Itoa(cfg.API.Port), "--pwd", cfg.API.Password, "--verbose")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			t.Error(err)
		}
		t.FailNow()
	}
	out, err := exec.Command(python, filepath.Join("testdata", "test.py"), "--port", strconv.Itoa(cfg.API.Port), "--pwd", cfg.API.Password).CombinedOutput()
	if err != nil {
		t.Error(err)
	}
	got := string(out)
	if runtime.GOOS == "windows" {
		got = strings.ReplaceAll(got, "\r", "")
	}
	want := bytes.Buffer{}
	_, mac := getMainAddr()
	if err := wantPython.Execute(&want, map[string]string{
		"GOOS":    runtime.GOOS,
		"Mac":     mac,
		"Version": version,
	}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want.String(), got); diff != "" {
		t.Errorf("python client mismatch (-want +got):\n%s", diff)
	}
}

func getFreePort(t *testing.T) int {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	networkBind = "127.0.0.1"
}
