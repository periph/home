// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"image"
	"image/color"

	"google.golang.org/protobuf/proto"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/devices/v3/apa102"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

func (n *Node) loadLightAPA102(ctx context.Context, cfg *config.Light) error {
	// TODO(maruel): Allow specifying port.
	p, err := spireg.Open("")
	if err != nil {
		return err
	}
	dev, err := apa102.New(p, &apa102.DefaultOpts)
	if err != nil {
		_ = p.Close()
		return err
	}
	return n.addEntity(ctx, &lightAPA102{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: lightComponent,
		},
		p:   p,
		d:   dev,
		img: image.NewNRGBA(image.Rect(0, 0, cfg.NumLEDs, 1)),
	})
}

type lightAPA102 struct {
	componentBase
	p   spi.PortCloser
	d   *apa102.Dev
	img *image.NRGBA
}

func (l *lightAPA102) Close() error {
	err := l.d.Halt()
	if err2 := l.p.Close(); err == nil {
		err = err2
	}
	return err
}

func (l *lightAPA102) init(ctx context.Context, n *Node) error {
	if err := l.componentBase.init(ctx, n); err != nil {
		return err
	}
	l.onNewState(&aioesphomeapi.LightStateResponse{
		Key:              l.key,
		ColorTemperature: float32(l.d.Temperature),
	})
	return nil
}

func (l *lightAPA102) describe() proto.Message {
	// TODO(maruel): Add mireds limits and effects.
	return &aioesphomeapi.ListEntitiesLightResponse{
		ObjectId:                 l.objectID,
		Key:                      l.key,
		Name:                     l.name,
		UniqueId:                 l.uniqueID,
		SupportsBrightness:       true,
		SupportsRgb:              true,
		SupportsWhiteValue:       false,
		SupportsColorTemperature: true,
		MinMireds:                0,
		MaxMireds:                0,
		Effects:                  nil,
	}
}

func (l *lightAPA102) lightCommand(in *aioesphomeapi.LightCommandRequest) error {
	if !in.State {
		_ = l.d.Halt()
	} else {
		// TODO(maruel): Proper rounding.
		l.d.Intensity = uint8(255. * in.Brightness)
		l.d.Temperature = uint16(in.ColorTemperature)
		c := color.NRGBA{uint8(255. * in.Red), uint8(255. * in.Green), uint8(255. * in.Blue), 255}
		for x := 0; x < 150; x++ {
			l.img.SetNRGBA(x, 0, c)
		}
		_ = l.d.Draw(l.d.Bounds(), l.img, image.Point{})
	}

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
