// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

func (n *Node) loadBinarySensorGPIO(ctx context.Context, cfg *config.BinarySensor) error {
	p := gpioreg.ByName(cfg.Pin.Number)
	if p == nil {
		return fmt.Errorf("unknown pin %q", cfg.Pin.Number)
	}
	pull := gpio.Float
	switch cfg.Pin.Mode {
	case config.Input:
	case config.InputPullup:
		pull = gpio.PullUp
	case config.InputPulldown:
		pull = gpio.PullDown
	case config.Analog:
		return errors.New("analog is not supported for binary sensor")
	case config.Output, config.OutputOpenDrain:
		return errors.New("output is not supported for binary sensor")
	default:
		return errors.New("unknown pin mode")
	}
	if err := p.In(pull, gpio.BothEdges); err != nil {
		return err
	}
	return n.addEntity(ctx, &binarySensorGPIO{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: binarySensorComponent,
		},
		deviceClass: cfg.DeviceClass,
		p:           p,
		inverted:    cfg.Pin.Inverted,
	})
}

type binarySensorGPIO struct {
	componentBase
	deviceClass string
	p           gpio.PinIO
	inverted    bool
	wg          sync.WaitGroup
	cancel      func()
}

func (b *binarySensorGPIO) Close() error {
	b.cancel()
	err := b.p.Halt()
	b.wg.Wait()
	return err
}

func (b *binarySensorGPIO) init(ctx context.Context, n *Node) error {
	if err := b.componentBase.init(ctx, n); err != nil {
		return err
	}

	l := bool(b.p.Read()) != b.inverted
	b.onNewState(&aioesphomeapi.BinarySensorStateResponse{
		Key:   b.key,
		State: l,
	})

	ctx, b.cancel = context.WithCancel(ctx)
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		<-ctx.Done()
		_ = b.p.Halt()
	}()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			// TODO(maruel): There's a bug in gpiotest that is hard to fix in v3:
			// Halt() doesn't unblock WaitForEdge(). Let's fix in v4.
			if !b.p.WaitForEdge(time.Second) {
				if ctx.Err() != nil {
					break
				}
				continue
			}
			if l2 := bool(b.p.Read()) != b.inverted; l2 != l {
				l = l2
				b.onNewState(&aioesphomeapi.BinarySensorStateResponse{
					Key:   b.key,
					State: bool(l) != b.inverted,
				})
			}
		}
		b.cancel()
	}()
	return nil
}

func (b *binarySensorGPIO) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesBinarySensorResponse{
		ObjectId:    b.objectID,
		Key:         b.key,
		Name:        b.name,
		UniqueId:    b.uniqueID,
		DeviceClass: b.deviceClass,
	}
}
