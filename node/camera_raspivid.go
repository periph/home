// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
	"periph.io/x/host/v3/rpi"
)

func (n *Node) loadCameraRaspivid(ctx context.Context, cfg *config.Camera) error {
	if cfg.Directory != "" {
		return errors.New("recording in a directory is not yet supported")
	}
	// It is recommended to use 720p or lower as it improves low light recording.
	return n.addEntity(ctx, &cameraRaspivid{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: cameraComponent,
		},
		directory: cfg.Directory,
		rotation:  cfg.Rotation,
		width:     1280,
		height:    720,
		quality:   60,
		fps:       1,
	})
}

type cameraRaspivid struct {
	componentBase
	directory string
	rotation  int
	width     int
	height    int
	quality   int
	fps       int

	cancel func()
	cmd    *exec.Cmd
}

func (c *cameraRaspivid) Close() error {
	c.cancel()
	_ = c.cmd.Wait()
	return nil
}

func (c *cameraRaspivid) init(ctx context.Context, n *Node) error {
	if err := c.componentBase.init(ctx, n); err != nil {
		return err
	}

	if c.directory != "" {
		// We use ffmpeg to compress the data on disk. Check that it's there on
		// startup so it doesn't fail later. Sadly running this command is
		// surprisingly slow, it takes nearly 2 seconds on my RPi Zero Wireless.
		out, err := exec.Command("ffmpeg", "-version").Output()
		if err != nil {
			return errors.New("please install ffmpeg: sudo apt install ffmpeg")
		}
		if rpi.Present() {
			// If on a Raspberry Pi, require hardware acceleration to be present.
			if !bytes.Contains(out, []byte("--enable-omx-rpi")) {
				return errors.New("please install ffmpeg with omx acceleration")
			}
		}

		if fi, err := os.Stat(c.directory); os.IsNotExist(err) {
			/* #nosec G301 */
			if err = os.MkdirAll(c.directory, 0o755); err != nil {
				return err
			}
		} else if !fi.IsDir() {
			return fmt.Errorf("exists but is not a directory: %s", c.directory)
		}
	}

	ctx, c.cancel = context.WithCancel(ctx)
	// We use raw format so we can embed a timestamp and compress to JPEG, since
	// it's what the ESPHome protocol expects.
	/* #nosec G204 */
	c.cmd = exec.CommandContext(
		ctx,
		"raspivid",
		"--nopreview",
		"--width", strconv.Itoa(c.width),
		"--height", strconv.Itoa(c.height),
		"--framerate", strconv.Itoa(c.fps),
		"--rotation", strconv.Itoa(c.rotation),
		// Run until canceled.
		"--timeout", "0",
		"--exposure", "auto",
		"--flicker", "off",
		"--awb", "auto",
		// Raw format.
		"--raw", "-",
		// While working in YUV420 saves bandwidth, it makes other things like
		// adding a timestamp much harder.
		//"--raw-format", "yuv",
		"--raw-format", "rgb",
	)
	c.cmd.Stdout = &rawRGB24JpegEncoder{
		onNewImage: func(b []byte) {
			log.Printf("next frame %d bytes", len(b))
			c.onNewState(&aioesphomeapi.CameraImageResponse{
				Key:  c.key,
				Data: b,
			})
		},
		width:   c.width,
		height:  c.height,
		quality: c.quality,
	}
	return c.cmd.Start()
}

func (c *cameraRaspivid) subscribe(ctx context.Context, cc clientConn) {
	log.Printf("camera cannot be subscribed to")
}

func (c *cameraRaspivid) cameraStream(ctx context.Context, cc clientConn, in *aioesphomeapi.CameraImageRequest) {
	// Reuse the componentBase fields that are used for subscribe(). It's fine
	// because "camera" doesn't support subscribe().
	log.Printf("cameraRaspivid(single=%t, stream=%t)", in.Single, in.Stream)

	// If there was a previous Stream = true message, a Single should cancel the stream. :/

	// Send initial message.
	c.mu.Lock()
	// Duplicate it, since we need to set Done:true if a stream is not requested.
	msg := proto.Clone(c.currentMsg).(*aioesphomeapi.CameraImageResponse)
	c.mu.Unlock()
	msg.Done = !in.Stream
	if err := cc.reply(msg); err != nil {
		return
	}
	if !in.Stream {
		return
	}

	k, ch, _ := c.register()
	defer c.unregister(k)
	// In ESPHome, it stops after 5 seconds. Not sure why.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := ctx.Done()
	for stop := false; !stop; {
		select {
		case msg := <-ch:
			if err := cc.reply(msg); err != nil {
				stop = true
			}
		case <-done:
			stop = true
		}
	}
}

func (c *cameraRaspivid) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesCameraResponse{
		ObjectId: c.objectID,
		Key:      c.key,
		Name:     c.name,
		UniqueId: c.uniqueID,
	}
}
