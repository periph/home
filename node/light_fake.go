// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

func (n *Node) loadLightFake(ctx context.Context, cfg *config.Light) error {
	return n.addEntity(ctx, &lightFake{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: lightComponent,
		},
	})
}

type lightFake struct {
	componentBase
}

func (l *lightFake) Close() error {
	return nil
}

func (l *lightFake) init(ctx context.Context, n *Node) error {
	if err := l.componentBase.init(ctx, n); err != nil {
		return err
	}
	l.onNewState(&aioesphomeapi.LightStateResponse{
		Key: l.key,
	})
	return nil
}

func (l *lightFake) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesLightResponse{
		ObjectId:                       l.objectID,
		Key:                            l.key,
		Name:                           l.name,
		UniqueId:                       l.uniqueID,
		LegacySupportsBrightness:       false,
		LegacySupportsRgb:              false,
		LegacySupportsWhiteValue:       false,
		LegacySupportsColorTemperature: false,
		MinMireds:                      0,
		MaxMireds:                      0,
		Effects:                        nil,
	}
}

func (l *lightFake) lightCommand(in *aioesphomeapi.LightCommandRequest) error {
	l.onNewState(&aioesphomeapi.LightStateResponse{
		Key:              l.key,
		State:            in.State,
		Brightness:       in.Brightness,
		Red:              in.Red,
		Green:            in.Green,
		Blue:             in.Blue,
		White:            in.White,
		ColorTemperature: in.ColorTemperature,
		Effect:           in.Effect,
	})
	return nil
}
