// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

func (n *Node) loadSensorWifiSignal(ctx context.Context, cfg *config.Sensor) error {
	if cfg.Temperature.Name != "" || cfg.Pressure.Name != "" || cfg.Humidity.Name != "" || cfg.Address != 0 {
		return errors.New("do not use temperature / pressure / humidity / address")
	}
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	if cfg.UpdateInterval == 0 {
		return errors.New("update_interval is required")
	}
	return n.addEntity(ctx, &sensorWifiSignal{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: sensorComponent,
		},
		update: cfg.UpdateInterval,
	})
}

type sensorWifiSignal struct {
	componentBase
	update time.Duration

	wg     sync.WaitGroup
	cancel func()
}

func (s *sensorWifiSignal) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *sensorWifiSignal) init(ctx context.Context, n *Node) error {
	if err := s.componentBase.init(ctx, n); err != nil {
		return err
	}

	v, err := s.read()
	if err != nil {
		return err
	}
	s.onNewState(&aioesphomeapi.SensorStateResponse{
		Key:   s.key,
		State: v,
	})

	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTimer(s.update)
		defer t.Stop()
		done := ctx.Done()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				v, err := s.read()
				if err != nil {
					return
				}
				s.onNewState(&aioesphomeapi.SensorStateResponse{
					Key:   s.key,
					State: v,
				})
			}
		}
	}()
	return nil
}

func (s *sensorWifiSignal) read() (float32, error) {
	if runtime.GOOS != "linux" {
		return 0, errors.New("please send a PR to implement wifi_signal on " + runtime.GOOS)
	}
	// Cheezy but avoid having to shell out anything or add another dependency.
	// Redo if it doesn't work well in practice.
	// Looks like this:
	// Inter-| sta-|   Quality        |   Discarded packets               | Missed | WE
	//  face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
	//   wlan0: 0000   50.  -60.  -256        0      0      0     14      0        0
	b, err := ioutil.ReadFile("/proc/net/wireless")
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 3 {
		return 0, errors.New("no wifi interface")
	}
	// Maybe find for wlan0?
	items := strings.Fields(lines[2])
	if len(items) != 11 {
		return 0, errors.New("unexpected /proc/net/wireless format")
	}
	v, err := strconv.ParseFloat(items[2], 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse RSSI in /proc/net/wireless: %w", err)
	}
	return -float32(v), nil
}

func (s *sensorWifiSignal) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesSensorResponse{
		ObjectId:          s.objectID,
		Key:               s.key,
		Name:              s.name,
		UniqueId:          s.uniqueID,
		Icon:              "mdi:wifi",
		UnitOfMeasurement: "dB",
	}
}
