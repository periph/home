// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

func (n *Node) loadCameraFake(ctx context.Context, cfg *config.Camera) error {
	// It is recommended to use 720p or lower as it improves low light recording.
	return n.addEntity(ctx, &cameraFake{
		componentBase: componentBase{
			name:          cfg.Name,
			componentType: cameraComponent,
		},
		directory: cfg.Directory,
		rotation:  cfg.Rotation,
		width:     320,
		height:    240,
		quality:   90,
		fps:       1,
	})
}

type cameraFake struct {
	componentBase
	directory string
	rotation  int
	width     int
	height    int
	quality   int
	fps       int

	index  int
	cancel func()
}

func (c *cameraFake) Close() error {
	c.cancel()
	return nil
}

func (c *cameraFake) init(ctx context.Context, n *Node) error {
	if err := c.componentBase.init(ctx, n); err != nil {
		return err
	}

	if c.directory != "" {
		if fi, err := os.Stat(c.directory); os.IsNotExist(err) {
			/* #nosec G301 */
			if err = os.MkdirAll(c.directory, 0o755); err != nil {
				return err
			}
		} else if !fi.IsDir() {
			return fmt.Errorf("exists but is not a directory: %s", c.directory)
		} else {
			names, err := filepath.Glob(filepath.Join(c.directory, "i*.jpg"))
			if err != nil {
				return nil
			}
			if len(names) != 0 {
				sort.Strings(names)
				for i := range names {
					n := filepath.Base(names[len(names)-1-i])
					if len(n) != 15 {
						continue
					}
					v, err := strconv.Atoi(n[1:11])
					if err != nil {
						continue
					}
					log.Printf("found index %d", v)
					c.index = v + 1
					break
				}
			}
		}
	}

	// Generate an image right away to simplify the code below.
	if err := c.genImage(time.Now()); err != nil {
		return err
	}

	ctx, c.cancel = context.WithCancel(ctx)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		t := time.NewTicker(time.Second / time.Duration(c.fps))
		defer t.Stop()
		done := ctx.Done()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				if err := c.genImage(now); err != nil {
					log.Printf("internal failure: %s", err)
				}
			}
		}
	}()
	return nil
}

func (c *cameraFake) genImage(now time.Time) error {
	img := genRGBATimeImg(c.width, c.height, now)
	buf := bytes.Buffer{}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: c.quality}); err != nil {
		return err
	}
	if err := c.onNewPicture(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (c *cameraFake) subscribe(ctx context.Context, cc clientConn) {
	log.Printf("camera cannot be subscribed to")
}

func (c *cameraFake) cameraStream(ctx context.Context, cc clientConn, in *aioesphomeapi.CameraImageRequest) {
	// Reuse the componentBase fields that are used for subscribe(). It's fine
	// because "camera" doesn't support subscribe().
	log.Printf("cameraFake(single=%t, stream=%t)", in.Single, in.Stream)

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

func (c *cameraFake) describe() proto.Message {
	return &aioesphomeapi.ListEntitiesCameraResponse{
		ObjectId: c.objectID,
		Key:      c.key,
		Name:     c.name,
		UniqueId: c.uniqueID,
	}
}

func (c *cameraFake) onNewPicture(b []byte) error {
	c.onNewState(&aioesphomeapi.CameraImageResponse{
		Key:  c.key,
		Data: b,
	})
	n := fmt.Sprintf("i%010d.jpg", c.index)
	//log.Printf("saving %s %d bytes", n, len(b))
	if c.directory != "" {
		/* #nosec G306 */
		if err := ioutil.WriteFile(filepath.Join(c.directory, n), b, 0o644); err != nil {
			return err
		}
	}
	c.index++
	return nil
}

func toUint8(i float64) uint8 {
	if i < 0.5 {
		return 0
	}
	if i > 254.5 {
		return 255
	}
	return uint8(i)
}

func linear(a, b color.RGBA, d float64) color.RGBA {
	return color.RGBA{
		R: toUint8(float64(a.R) + d*float64(b.R-a.R)),
		G: toUint8(float64(a.G) + d*float64(b.G-a.G)),
		B: toUint8(float64(a.B) + d*float64(b.B-a.B)),
		A: 255,
	}
}

func linearGradient(img *image.RGBA, a, b color.RGBA) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.SetRGBA(x, y, linear(a, b, float64(x)/float64(w)))
		}
	}
}

// genRGBATimeImg generates a simple image with time.
func genRGBATimeImg(w, h int, now time.Time) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	linearGradient(img, color.RGBA{0, 0, 128, 255}, color.RGBA{72, 0, 0, 255})
	addTimestamp(img, color.RGBA{255, 255, 255, 255}, now)
	return img
}
