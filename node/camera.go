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
	"image/draw"
	"image/jpeg"
	"log"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"periph.io/x/home/node/config"
)

func (n *Node) loadCamera(ctx context.Context, cfg *config.Camera) error {
	log.Printf("loading camera %s", cfg.Platform)
	switch cfg.Platform {
	case "fake":
		if err := n.loadCameraFake(ctx, cfg); err != nil {
			return fmt.Errorf("sensor(%s): %w", cfg.Platform, err)
		}
		return nil
	case "raspivid":
		if err := n.loadCameraRaspivid(ctx, cfg); err != nil {
			return fmt.Errorf("sensor(%s): %w", cfg.Platform, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown platform %q", cfg.Platform)
	}
}

// rawRGB24JpegEncoder takes a raw RGB24 stream and encodes it to JPEG.
type rawRGB24JpegEncoder struct {
	onNewImage func(b []byte)
	buf        bytes.Buffer
	width      int
	height     int
	quality    int
}

func (r *rawRGB24JpegEncoder) Write(b []byte) (int, error) {
	// TODO(maruel): send the data into ffmpeg for it to split into mpegts with a
	// index.m3u8. This enables serving the directory as-is for visioning history
	// via a web browser.
	_, _ = r.buf.Write(b)
	f := r.width * r.height * 3
	for r.buf.Len() >= f {
		// Warning: this goes in the slow code path for Encode(). Do a benchmark to
		// compare with image.RGBA which is more optimized.
		img := imageRGB24{w: r.width, h: r.height, pix: r.buf.Bytes()[:f]}
		addTimestamp(&img, color.RGBA{200, 100, 0, 255}, time.Now())
		buf := bytes.Buffer{}
		if err := jpeg.Encode(&buf, &img, &jpeg.Options{Quality: r.quality}); err != nil {
			log.Printf("jpeg failure: %s", err)
			return len(b), nil
		}
		r.onNewImage(buf.Bytes())
		// Advance the buffer.
		r.buf.Next(f)
	}
	return len(b), nil
}

type imageRGB24 struct {
	w   int
	h   int
	pix []byte
}

func (i *imageRGB24) ColorModel() color.Model {
	return color.NRGBAModel
}

func (i *imageRGB24) Bounds() image.Rectangle {
	return image.Rect(0, 0, i.w, i.h)
}

func (i *imageRGB24) At(x, y int) color.Color {
	o := x*3 + (i.w * y * 3)
	return color.NRGBA{R: i.pix[o], G: i.pix[o+1], B: i.pix[o+2], A: 255}
}

func (i *imageRGB24) Set(x, y int, c color.Color) {
	o := x*3 + (i.w * y * 3)
	r, g, b, _ := c.RGBA()
	i.pix[o] = uint8(r >> 8)
	i.pix[o+1] = uint8(g >> 8)
	i.pix[o+2] = uint8(b >> 8)
}

/*
// rawYUV420JpegEncoder takes a raw YCbCr / YUV420 stream and encodes it to JPEG.
type rawYUV420JpegEncoder struct {
	onNewImage func(b []byte)
	buf        bytes.Buffer
	width      int
	height     int
	quality    int
}

func (r *rawYUV420JpegEncoder) Write(b []byte) (int, error) {
	// TODO(maruel): send the data into ffmpeg for it to split into mpegts with a
	// index.m3u8. This enables serving the directory as-is for visioning history
	// via a web browser.
	_, _ = r.buf.Write(b)
	f := (r.width*r.height*3 + 1) / 2
	for r.buf.Len() >= f {
		img := rawYUV420ToImage(r.width, r.height, r.buf.Bytes()[:f])
		buf := bytes.Buffer{}
		// jpeg.Encode() is specifically optimized for image.YCbCr.
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: r.quality}); err != nil {
			log.Printf("jpeg failure: %s", err)
			return len(b), nil
		}
		r.onNewImage(buf.Bytes())
		// Advance the buffer.
		r.buf.Next(f)
	}
	return len(b), nil
}

func rawYUV420ToImage(w, h int, raw []byte) *image.YCbCr {
	cw := w / 2
	ch := h / 2
	i0 := w*h + 0*cw*ch
	i1 := w*h + 1*cw*ch
	i2 := w*h + 2*cw*ch
	return &image.YCbCr{
		Y:              raw[:i0:i0],
		Cb:             raw[i0:i1:i1],
		Cr:             raw[i1:i2:i2],
		YStride:        w,
		CStride:        cw,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, w, h),
	}
}
*/

// addTimestamp adds the time `now` to an image.
func addTimestamp(img draw.Image, c color.RGBA, now time.Time) {
	// TODO(maruel): Make the text size proportional to the image width.
	// TODO(maruel): Align top right.
	b := img.Bounds()
	x := b.Max.X
	y := b.Max.Y
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(x / 3 * 64),
			Y: fixed.Int26_6(y / 3 * 64),
		},
	}
	d.DrawString(now.Format("2006-01-02 15:04:05"))
}
