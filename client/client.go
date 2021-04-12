// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package client implements the client code for an esphome node.
package client

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"
)

// Found is a printer found on the network.
type Found struct {
	Name     string
	Hostname string
	IP       net.IP
	Port     int
	Text     []string

	_ struct{}
}

func (f *Found) String() string {
	return fmt.Sprintf("%s (%s / %s:%d): %s", f.Name, f.Hostname, f.IP, f.Port, f.Text)
}

// Search searches for devices on the local network that implements the esphome
// protocol.
//
// This is done via zeroconf, which use 224.0.0.251 on port 5353.
func Search(ctx context.Context, first bool) ([]*Found, error) {
	var cancel func()
	if first {
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}
	r, err := zeroconf.NewResolver()
	if err != nil {
		return nil, err
	}
	c := make(chan *zeroconf.ServiceEntry)
	var out []*Found
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for e := range c {
			if first && len(out) != 0 {
				continue
			}
			f := &Found{
				Name:     e.Instance,
				Hostname: strings.TrimRight(e.HostName, "."),
				Port:     e.Port,
				Text:     e.Text,
			}
			if len(e.AddrIPv4) != 0 {
				f.IP = e.AddrIPv4[0]
			} else if len(e.AddrIPv6) != 0 {
				f.IP = e.AddrIPv6[0]
			}
			out = append(out, f)
			if first {
				cancel()
			}
		}
	}()

	if err = r.Browse(ctx, "_esphomelib._tcp", "local.", c); err != nil {
		return nil, err
	}
	<-ctx.Done()
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}
