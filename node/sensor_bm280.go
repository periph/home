// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/devices/v3/bmxx80"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

// loadSensorBMxx80 loads the sensor and each component separately.
func (n *Node) loadSensorBMxx80(ctx context.Context, cfg *config.Sensor) error {
	if cfg.Temperature.Name == "" && cfg.Pressure.Name == "" && cfg.Humidity.Name == "" {
		return errors.New("specify a name for at least one sensor")
	}
	if cfg.UpdateInterval == 0 {
		return errors.New("update_interval is required")
	}
	if cfg.Name != "" {
		return errors.New("name is not supported")
	}
	d := &devBMxx80{
		update: cfg.UpdateInterval,
	}

	opts := bmxx80.Opts{
		Temperature: bmxx80.O16x,
		Pressure:    bmxx80.O16x,
		Humidity:    bmxx80.O16x,
	}
	if cfg.Pressure.Name == "" {
		opts.Pressure = bmxx80.Off
	}
	if cfg.Humidity.Name == "" {
		opts.Humidity = bmxx80.Off
	}

	// TODO(maruel): Define which SPI or I²C bus to use.
	if cfg.Address != 0 {
		p, err := i2creg.Open("")
		if err != nil {
			return err
		}
		dev, err := bmxx80.NewI2C(p, uint16(cfg.Address), &opts)
		if err != nil {
			_ = p.Close()
			return err
		}
		d.bus = p
		d.d = dev
	} else {
		p, err := spireg.Open("")
		if err != nil {
			return err
		}
		dev, err := bmxx80.NewSPI(p, &opts)
		if err != nil {
			_ = p.Close()
			return err
		}
		d.bus = p
		d.d = dev
	}

	if err := d.init(ctx); err != nil {
		_ = d.Close()
		return err
	}

	// Add one component per activated sensor.
	first := true
	if cfg.Temperature.Name != "" {
		c := &sensorBMxx80{
			componentBase: componentBase{
				name:          cfg.Temperature.Name,
				componentType: sensorComponent,
			},
			d:        d,
			unit:     "°C",
			devcls:   "temperature",
			accuracy: 1,
			first:    first,
		}
		if err := n.addEntity(ctx, c); err != nil {
			_ = d.Close()
			return err
		}
		d.temp = c
		first = false
	}
	if cfg.Pressure.Name != "" {
		c := &sensorBMxx80{
			componentBase: componentBase{
				name:          cfg.Pressure.Name,
				componentType: sensorComponent,
			},
			d:        d,
			unit:     "kPa",
			devcls:   "pressure",
			accuracy: 2,
			first:    first,
		}
		if err := n.addEntity(ctx, c); err != nil {
			_ = d.Close()
			return err
		}
		d.pres = c
		first = false
	}
	if cfg.Humidity.Name != "" {
		c := &sensorBMxx80{
			componentBase: componentBase{
				name:          cfg.Humidity.Name,
				componentType: sensorComponent,
			},
			d:        d,
			unit:     "%",
			devcls:   "humidity",
			accuracy: 1,
			first:    first,
		}
		if err := n.addEntity(ctx, c); err != nil {
			_ = d.Close()
			return err
		}
		d.humi = c
	}
	return nil
}

type sensorBMxx80 struct {
	componentBase
	d        *devBMxx80
	unit     string
	devcls   string
	accuracy int32
	first    bool
}

func (s *sensorBMxx80) Close() error {
	// There's a mismatch here because there's up to 3 sensors but one device
	// handle. Have the first Close close them all. Since it only happens at
	// shutdown, it's "fine".
	if s.first {
		return s.d.Close()
	}
	return nil
}

func (s *sensorBMxx80) describe() proto.Message {
	// TODO(maruel): Add icon, tweak values.
	return &aioesphomeapi.ListEntitiesSensorResponse{
		ObjectId:          s.objectID,
		Key:               s.key,
		Name:              s.name,
		UniqueId:          s.uniqueID,
		Icon:              "",
		UnitOfMeasurement: s.unit,
		AccuracyDecimals:  s.accuracy,
		ForceUpdate:       false,
		DeviceClass:       s.devcls,
	}
}

// devBMxx80 is the underlying connection for the sensors.
type devBMxx80 struct {
	bus    io.Closer
	d      *bmxx80.Dev
	update time.Duration
	temp   *sensorBMxx80
	pres   *sensorBMxx80
	humi   *sensorBMxx80
}

func (d *devBMxx80) Close() error {
	err := d.d.Halt()
	if err2 := d.bus.Close(); err == nil {
		err = err2
	}
	return err
}

func (d *devBMxx80) init(cfx context.Context) error {
	ch, err := d.d.SenseContinuous(d.update)
	if err != nil {
		return err
	}
	go func() {
		for e := range ch {
			d.send(e)
		}
	}()
	return nil
}

func (d *devBMxx80) send(e physic.Env) {
	if d.temp != nil {
		d.temp.onNewState(&aioesphomeapi.SensorStateResponse{
			Key:   d.temp.key,
			State: float32(e.Temperature.Celsius()),
		})
	}
	if d.pres != nil {
		d.pres.onNewState(&aioesphomeapi.SensorStateResponse{
			Key:   d.pres.key,
			State: float32(e.Pressure) / float32(physic.KiloPascal),
		})
	}
	if d.humi != nil {
		d.humi.onNewState(&aioesphomeapi.SensorStateResponse{
			Key:   d.humi.key,
			State: float32(e.Humidity) / float32(physic.PercentRH),
		})
	}
}
