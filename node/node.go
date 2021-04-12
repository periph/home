// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/grandcat/zeroconf"
	"google.golang.org/protobuf/proto"
	"periph.io/x/home/node/config"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

const version = "0.1"

// New loads a configuration and instantiate a node.
func New(ctx context.Context, cfg *config.Root) (*Node, error) {
	ifa, mac := getMainAddr()
	n := &Node{
		cfg:    cfg,
		lookup: map[uint32]component{},
		mac:    mac,
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	// Parses all the sensors.
	for i := range cfg.BinarySensors {
		if err = n.loadBinarySensor(ctx, &cfg.BinarySensors[i]); err != nil {
			// Since we're partially initialized, take the time to close the
			// components that were initialized.
			_ = n.Close()
			return nil, err
		}
	}
	for i := range cfg.Sensors {
		if err = n.loadSensor(ctx, &cfg.Sensors[i]); err != nil {
			// Since we're partially initialized, take the time to close the
			// components that were initialized.
			_ = n.Close()
			return nil, err
		}
	}
	for i := range cfg.Lights {
		if err = n.loadLight(ctx, &cfg.Lights[i]); err != nil {
			// Since we're partially initialized, take the time to close the
			// components that were initialized.
			_ = n.Close()
			return nil, err
		}
	}
	for i := range cfg.Cameras {
		if err = n.loadCamera(ctx, &cfg.Cameras[i]); err != nil {
			// Since we're partially initialized, take the time to close the
			// components that were initialized.
			_ = n.Close()
			return nil, err
		}
	}

	// Start the native API server.
	port := 6053
	if n.cfg.API.IsPresent {
		if port = n.cfg.API.Port; port == 0 {
			port = 6053
		}
		if err := n.apiServer(ctx, port); err != nil {
			_ = n.Close()
			return nil, fmt.Errorf("failed to start api server: %w", err)
		}
	}

	// Make the device discoverable via eroconf but not in unit test because it
	// will throw a firewall prompt on Windows.
	if networkBind == "" {
		text := []string{
			"address=" + hostname + ".local",
			"version=" + version,
		}
		if n.mac != "" {
			// Not sure of the value here.
			text = append(text, "mac="+strings.ReplaceAll(n.mac, ":", ""))
		}
		// TODO(maruel): What about when the native api is not enabled? Right now
		// it exposes an invalid port.
		log.Printf("Advertizing via zeroconf %v", text)
		var ifas []net.Interface
		if ifa != nil {
			ifas = append(ifas, *ifa)
		}
		zc, err := zeroconf.Register(cfg.PeriphHome.Name, "_esphomelib._tcp", "local.", port, text, ifas)
		if err != nil {
			_ = n.Close()
			return nil, fmt.Errorf("failed to advertise with zeroconf: %w", err)
		}
		n.zc = zc
	}
	return n, nil
}

// Node is the periphhome node.
type Node struct {
	cfg *config.Root
	mac string

	// Components.
	entities []component
	// For native API requests.
	lookup map[uint32]component

	// Discovery.
	zc *zeroconf.Server

	// API server.
	ln net.Listener
	wg sync.WaitGroup
}

// Close stops all the sensors, devices and close the API server as relevant.
func (n *Node) Close() error {
	// Close in the reverse order of New(). Has to handle partially initialized
	// object when New() is failing.
	if n.zc != nil {
		log.Printf("shutting down zeroconf")
		n.zc.Shutdown()
	}
	var err error
	if n.ln != nil {
		log.Printf("shutting down api")
		err = n.ln.Close()
	}
	for i := range n.entities {
		log.Printf("closing component %s", n.entities[i].getName())
		if err2 := n.entities[i].Close(); err == nil {
			err = err2
		}
	}
	log.Printf("waiting for goroutines")
	if os.Getenv("GOTRACEBACK") == "all" {
		// This code exists to catch when there's a shutdown bug.
		t := time.AfterFunc(time.Minute, func() {
			panic("Took too long to shutdown, panicking")
		})
		n.wg.Wait()
		t.Stop()
	} else {
		n.wg.Wait()
	}
	return err
}

func (n *Node) addEntity(ctx context.Context, c component) error {
	if err := c.init(ctx, n); err != nil {
		return err
	}
	n.entities = append(n.entities, c)
	n.lookup[c.getHash()] = c
	return nil
}

// apiServer starts the API server as documented at
// https://esphome.io/components/api.html and implemented at
// https://github.com/esphome/aioesphomeapi.
func (n *Node) apiServer(ctx context.Context, port int) error {
	log.Printf("loading API server on port %d", port)
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf("%s:%d", networkBind, port))
	if err != nil {
		return err
	}
	logf("listening on %s", ln.Addr())

	n.ln = ln
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.apiServerLoop(ctx)
	}()
	return nil
}

func (n *Node) apiServerLoop(ctx context.Context) {
	for {
		c, err := n.ln.Accept()
		if err != nil {
			return
		}
		logf("New connection: %s", c.RemoteAddr())
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			(&conn{c: c, n: n}).handleConnection(ctx)
		}()
	}
}

type component interface {
	Close() error
	init(ctx context.Context, n *Node) error
	getName() string
	getUniqueID() string
	getHash() uint32
	getType() componentType
	describe() proto.Message
	// subscribe shall block and send updates until the context is closed.
	subscribe(ctx context.Context, c clientConn)
	// cameraStream shall block and send pictures until the context is closed.
	cameraStream(ctx context.Context, c clientConn, in *aioesphomeapi.CameraImageRequest)
	climateCommand(in *aioesphomeapi.ClimateCommandRequest) error
	coverCommand(in *aioesphomeapi.CoverCommandRequest) error
	fanCommand(in *aioesphomeapi.FanCommandRequest) error
	lightCommand(in *aioesphomeapi.LightCommandRequest) error
	switchCommand(in *aioesphomeapi.SwitchCommandRequest) error
}

type componentBase struct {
	name          string
	componentType componentType

	// Calculated in init():
	objectID string
	uniqueID string
	key      uint32

	// For subscriptions.
	mu         sync.Mutex
	nextChKey  int
	ch         map[int]chan proto.Message
	currentMsg proto.Message
}

func (c *componentBase) init(ctx context.Context, n *Node) error {
	if c.name == "" {
		return errors.New("internal error: name not set")
	}
	if c.componentType == "" {
		return errors.New("internal error: componentType not set")
	}

	c.objectID = strings.Map(func(r rune) rune {
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') || r == '-' || r == '_' {
			return r
		}
		if 'A' <= r && r <= 'Z' {
			return unicode.ToLower(r)
		}
		return -1
	}, c.name)
	if c.objectID == "" {
		return errors.New("internal error: objectID is empty")
	}
	// Default uniqueID is Node.Name + component type + object_id. Some
	// override with the mac address plus something related to the component.
	c.uniqueID = n.cfg.PeriphHome.Name + string(c.componentType) + c.objectID

	h := fnv.New32()
	if _, err := h.Write([]byte(c.objectID)); err != nil {
		return err
	}
	if c.key = h.Sum32(); c.key == 0 {
		// I observed that if the hash value is 0, it is replaced with 1 by the
		// client.
		c.key = 1
	}
	c.ch = map[int]chan proto.Message{}
	return nil
}

func (c *componentBase) getName() string {
	return c.name
}

func (c *componentBase) getUniqueID() string {
	return c.uniqueID
}

func (c *componentBase) getHash() uint32 {
	return c.key
}

func (c *componentBase) getType() componentType {
	return c.componentType
}

func (c *componentBase) cameraStream(ctx context.Context, cc clientConn, in *aioesphomeapi.CameraImageRequest) {
	log.Printf("%s is no camera", c.name)
}

func (c *componentBase) climateCommand(in *aioesphomeapi.ClimateCommandRequest) error {
	return fmt.Errorf("%s is no climate", c.name)
}

func (c *componentBase) coverCommand(in *aioesphomeapi.CoverCommandRequest) error {
	return fmt.Errorf("%s is no cover", c.name)
}

func (c *componentBase) fanCommand(in *aioesphomeapi.FanCommandRequest) error {
	return fmt.Errorf("%s is no fan", c.name)
}

func (c *componentBase) lightCommand(in *aioesphomeapi.LightCommandRequest) error {
	return fmt.Errorf("%s is no light", c.name)
}

func (c *componentBase) switchCommand(in *aioesphomeapi.SwitchCommandRequest) error {
	return fmt.Errorf("%s is no switch", c.name)
}

func (c *componentBase) register() (int, chan proto.Message, proto.Message) {
	// It's not awesome, we should have proper locking semantics instead.
	ch := make(chan proto.Message, 8)
	c.mu.Lock()
	k := c.nextChKey
	c.nextChKey++
	c.ch[k] = ch
	cur := c.currentMsg
	c.mu.Unlock()
	return k, ch, cur
}

func (c *componentBase) unregister(k int) {
	c.mu.Lock()
	delete(c.ch, k)
	c.mu.Unlock()
}

// onNewState sends the state update it to every subscription.
func (c *componentBase) onNewState(msg proto.Message) {
	c.mu.Lock()
	c.currentMsg = msg
	for _, ch := range c.ch {
		ch <- msg
	}
	c.mu.Unlock()
}

func (c *componentBase) subscribe(ctx context.Context, cc clientConn) {
	k, ch, msg := c.register()
	defer c.unregister(k)
	// Send initial message.
	if err := cc.reply(msg); err != nil {
		return
	}

	done := ctx.Done()
	for stop := false; !stop; {
		select {
		case msg = <-ch:
			if err := cc.reply(msg); err != nil {
				stop = true
			}
		case <-done:
			stop = true
		}
	}
}

type componentType string

const (
	binarySensorComponent componentType = "binary_sensor"
	cameraComponent       componentType = "camera"
	climateComponent      componentType = "climate"
	coverComponent        componentType = "cover"
	fanComponent          componentType = "fan"
	lightComponent        componentType = "light"
	sensorComponent       componentType = "sensor"
	switchComponent       componentType = "switch"
)

// clientConn is used by interface component.
type clientConn interface {
	reply(msg proto.Message) error
}

// getMainAddr returns the first IP and mac addresses that are not a loopback
// and support multicast.
//
// This assumes that lesser use network adapters like docker and tailscale are
// after the base one. Still it's not clear which one is useful to return so
// this code will likely have to change.
func getMainAddr() (*net.Interface, string) {
	ifas, _ := net.Interfaces()
	for _, ifa := range ifas {
		if ifa.Flags&net.FlagLoopback != 0 || ifa.Flags&net.FlagUp == 0 || ifa.Flags&net.FlagMulticast == 0 {
			continue
		}
		mac := ifa.HardwareAddr.String()
		if mac == "" {
			continue
		}
		addrs, err := ifa.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		return &ifa, mac
	}
	return nil, ""
}

// networkBind is set in test so the temporary server is bound on the
// local loop network.
var networkBind = ""
