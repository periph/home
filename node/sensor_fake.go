// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

// loadSensorFake is essentially uptime but only for the node itself.
func (n *Node) loadSensorFake(ctx context.Context, cfg *config.Sensor) error {
	if cfg.Temperature.Name != "" || cfg.Pressure.Name != "" || cfg.Humidity.Name != "" || cfg.Address != 0 {
		return errors.New("do not use temperature / pressure / humidity / address")
	}
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	if cfg.UpdateInterval == 0 {
		return errors.New("update_interval is required")
	}
	return n.addEntity(ctx, &sensorFake{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: sensorComponent,
		},
		update: cfg.UpdateInterval,
	})
}

type sensorFake struct {
	componentBase
	update time.Duration

	wg     sync.WaitGroup
	cancel func()
}

func (s *sensorFake) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *sensorFake) init(ctx context.Context, n *Node) error {
	if err := s.componentBase.init(ctx, n); err != nil {
		return err
	}

	s.onNewState(&aioesphomeapi.SensorStateResponse{
		Key:   s.key,
		State: 1.0,
	})

	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// The VMs on github actions can take easily 17 seconds between starting
		// the python testdata/test.py client and returning the value here. This is
		// incredibly racy.
		t := time.NewTicker(s.update)
		defer t.Stop()
		done := ctx.Done()
		for start := time.Now(); ; {
			select {
			case <-done:
				return
			case <-t.C:
				s.onNewState(&aioesphomeapi.SensorStateResponse{
					Key:   s.key,
					State: float32(time.Since(start)) / float32(time.Second),
				})
			}
		}
	}()
	return nil
}

func (s *sensorFake) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesSensorResponse{
		ObjectId:          s.objectID,
		Key:               s.key,
		Name:              s.name,
		UniqueId:          s.uniqueID,
		Icon:              "mdi:exclamation",
		UnitOfMeasurement: "",
	}
}
