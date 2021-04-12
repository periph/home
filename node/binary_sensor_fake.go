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

func (n *Node) loadBinarySensorFake(ctx context.Context, cfg *config.BinarySensor) error {
	if cfg.Pin.Number != "" {
		return errors.New("fake doesn't support pin number")
	}
	return n.addEntity(ctx, &binarySensorFake{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: binarySensorComponent,
		},
		deviceClass: cfg.DeviceClass,
	})
}

type binarySensorFake struct {
	componentBase
	deviceClass string

	wg     sync.WaitGroup
	cancel func()
}

func (b *binarySensorFake) Close() error {
	b.cancel()
	b.wg.Wait()
	return nil
}

func (b *binarySensorFake) init(ctx context.Context, n *Node) error {
	if err := b.componentBase.init(ctx, n); err != nil {
		return err
	}

	b.onNewState(&aioesphomeapi.BinarySensorStateResponse{
		Key:   b.key,
		State: false,
	})

	ctx, b.cancel = context.WithCancel(ctx)
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		// The VMs on github actions can take easily 17 seconds between starting
		// the python testdata/test.py client and returning the value here. This is
		// incredibly racy. Hardcode 60 seconds here for now but we may need to
		// hook it in api_test.go.
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		done := ctx.Done()
		for l := true; ; l = !l {
			select {
			case <-done:
				return
			case <-t.C:
				b.onNewState(&aioesphomeapi.BinarySensorStateResponse{
					Key:   b.key,
					State: l,
				})
			}
		}
	}()
	return nil
}

func (b *binarySensorFake) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesBinarySensorResponse{
		ObjectId:    b.objectID,
		Key:         b.key,
		Name:        b.name,
		UniqueId:    b.uniqueID,
		DeviceClass: b.deviceClass,
	}
}
