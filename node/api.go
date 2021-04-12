// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package node implements the common code to present itself as an esphome
// node.
//
// The primary use case is for integration into Home Assistant.
package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"runtime"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
	"periph.io/x/home/thirdparty/aioesphomeapi"
)

// conn is a native API TCP connection.
type conn struct {
	c net.Conn
	n *Node
}

func (c *conn) handleConnection(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.c.Close()
	type msg struct {
		id  int
		raw []byte
		err error
	}
	done := ctx.Done()
	onMsg := make(chan msg, 1)
	for {
		go func() {
			id, raw, err := readMsg(c.c)
			onMsg <- msg{id, raw, err}
		}()
		select {
		case <-done:
			if err := c.reply(&aioesphomeapi.DisconnectRequest{}); err != nil {
				// Then don't wait for a reply.
				return
			}
			// Wait for the reply which should be a DisconnectResponse.
			select {
			case <-onMsg:
			case <-time.After(5 * time.Second):
			}
			return
		case m := <-onMsg:
			if m.err != nil {
				if !isErrEOF(m.err) {
					log.Printf("readMsg: %s", m.err)
				} else {
					logf("readMsg: %s", m.err)
				}
				return
			}
			if err := c.handleRPC(ctx, m.id, m.raw); err != nil {
				if !isErrEOF(err) {
					log.Printf("handleRPC: %s", err)
				} else {
					logf("handleRPC: %s", err)
				}
				return
			}
		}
	}
}

func (c *conn) handleRPC(ctx context.Context, id int, msg []byte) error {
	// It'd be nicer to use reflection but it'd be slower. Since it's all
	// immutable constants, it's not that much a big deal.
	t := requests[id]
	v := reflect.New(t).Interface().(proto.Message)
	if err := proto.Unmarshal(msg, v); err != nil {
		return err
	}
	logf("handleRPC(%T)", v)
	switch id {
	case 1:
		return c.Hello(v.(*aioesphomeapi.HelloRequest))
	case 3:
		return c.Connect(v.(*aioesphomeapi.ConnectRequest))
	case 5:
		return c.Disconnect(v.(*aioesphomeapi.DisconnectRequest))
	case 7:
		return c.Ping(v.(*aioesphomeapi.PingRequest))
	case 9:
		return c.DeviceInfo(v.(*aioesphomeapi.DeviceInfoRequest))
	case 11:
		return c.ListEntities(v.(*aioesphomeapi.ListEntitiesRequest))
	case 20:
		return c.SubscribeStates(ctx, v.(*aioesphomeapi.SubscribeStatesRequest))
	case 28:
		return c.SubscribeLogs(v.(*aioesphomeapi.SubscribeLogsRequest))
	case 30:
		return c.CoverCommand(v.(*aioesphomeapi.CoverCommandRequest))
	case 31:
		return c.FanCommand(v.(*aioesphomeapi.FanCommandRequest))
	case 32:
		return c.LightCommand(v.(*aioesphomeapi.LightCommandRequest))
	case 33:
		return c.SwitchCommand(v.(*aioesphomeapi.SwitchCommandRequest))
	case 34:
		return c.SubscribeHomeassistantServices(v.(*aioesphomeapi.SubscribeHomeassistantServicesRequest))
	case 36:
		return c.GetTime(v.(*aioesphomeapi.GetTimeRequest))
	case 38:
		return c.SubscribeHomeAssistantStates(v.(*aioesphomeapi.SubscribeHomeAssistantStatesRequest))
	case 40:
		return c.OnHomeAssistantState(v.(*aioesphomeapi.HomeAssistantStateResponse))
	case 42:
		return c.ExecuteService(v.(*aioesphomeapi.ExecuteServiceRequest))
	case 45:
		return c.CameraImage(ctx, v.(*aioesphomeapi.CameraImageRequest))
	case 48:
		return c.ClimateCommand(v.(*aioesphomeapi.ClimateCommandRequest))
	default:
		return fmt.Errorf("internal error: implement %d", id)
	}
}

// requests maps the incoming message ID from the client to the right type for
// deserialization.
//
// This includes all messages in api.proto marked with SOURCE_CLIENT or
// SOURCE_BOTH.
var requests = map[int]reflect.Type{
	1:  reflect.TypeOf(aioesphomeapi.HelloRequest{}),
	3:  reflect.TypeOf(aioesphomeapi.ConnectRequest{}),
	5:  reflect.TypeOf(aioesphomeapi.DisconnectRequest{}),
	6:  reflect.TypeOf(aioesphomeapi.DisconnectResponse{}), // SOURCE_BOTH
	7:  reflect.TypeOf(aioesphomeapi.PingRequest{}),
	8:  reflect.TypeOf(aioesphomeapi.PingResponse{}), // SOURCE_BOTH
	9:  reflect.TypeOf(aioesphomeapi.DeviceInfoRequest{}),
	11: reflect.TypeOf(aioesphomeapi.ListEntitiesRequest{}),
	20: reflect.TypeOf(aioesphomeapi.SubscribeStatesRequest{}),
	28: reflect.TypeOf(aioesphomeapi.SubscribeLogsRequest{}),
	30: reflect.TypeOf(aioesphomeapi.CoverCommandRequest{}),
	31: reflect.TypeOf(aioesphomeapi.FanCommandRequest{}),
	32: reflect.TypeOf(aioesphomeapi.LightCommandRequest{}),
	33: reflect.TypeOf(aioesphomeapi.SwitchCommandRequest{}),
	34: reflect.TypeOf(aioesphomeapi.SubscribeHomeassistantServicesRequest{}),
	36: reflect.TypeOf(aioesphomeapi.GetTimeRequest{}),
	37: reflect.TypeOf(aioesphomeapi.GetTimeResponse{}), // SOURCE_BOTH
	38: reflect.TypeOf(aioesphomeapi.SubscribeHomeAssistantStatesRequest{}),
	40: reflect.TypeOf(aioesphomeapi.HomeAssistantStateResponse{}), // Reverse of normal convention
	42: reflect.TypeOf(aioesphomeapi.ExecuteServiceRequest{}),
	45: reflect.TypeOf(aioesphomeapi.CameraImageRequest{}),
	48: reflect.TypeOf(aioesphomeapi.ClimateCommandRequest{}),
}

// getID returns the ID to send a package back to the client.
//
// This includes all messages in api.proto marked with SOURCE_SERVER or
// SOURCE_BOTH.
func getID(msg interface{}) int {
	switch msg.(type) {
	case *aioesphomeapi.HelloResponse:
		return 2
	case *aioesphomeapi.ConnectResponse:
		return 4
	case *aioesphomeapi.DisconnectRequest: // SOURCE_BOTH
		return 5
	case *aioesphomeapi.DisconnectResponse:
		return 6
	case *aioesphomeapi.PingRequest: // SOURCE_BOTH
		return 7
	case *aioesphomeapi.PingResponse:
		return 8
	case *aioesphomeapi.DeviceInfoResponse:
		return 10
	case *aioesphomeapi.ListEntitiesBinarySensorResponse:
		return 12
	case *aioesphomeapi.ListEntitiesCoverResponse:
		return 13
	case *aioesphomeapi.ListEntitiesFanResponse:
		return 14
	case *aioesphomeapi.ListEntitiesLightResponse:
		return 15
	case *aioesphomeapi.ListEntitiesSensorResponse:
		return 16
	case *aioesphomeapi.ListEntitiesSwitchResponse:
		return 17
	case *aioesphomeapi.ListEntitiesTextSensorResponse:
		return 18
	case *aioesphomeapi.ListEntitiesDoneResponse:
		return 19
	case *aioesphomeapi.BinarySensorStateResponse:
		return 21
	case *aioesphomeapi.CoverStateResponse:
		return 22
	case *aioesphomeapi.FanStateResponse:
		return 23
	case *aioesphomeapi.LightStateResponse:
		return 24
	case *aioesphomeapi.SensorStateResponse:
		return 25
	case *aioesphomeapi.SwitchStateResponse:
		return 26
	case *aioesphomeapi.TextSensorStateResponse:
		return 27
	case *aioesphomeapi.SubscribeLogsResponse:
		return 29
	case *aioesphomeapi.HomeassistantServiceResponse:
		return 35
	case *aioesphomeapi.GetTimeRequest: // SOURCE_BOTH
		return 36
	case *aioesphomeapi.GetTimeResponse:
		return 37
	case *aioesphomeapi.SubscribeHomeAssistantStateResponse:
		return 39
	case *aioesphomeapi.ListEntitiesServicesResponse:
		return 41
	case *aioesphomeapi.ListEntitiesCameraResponse:
		return 43
	case *aioesphomeapi.CameraImageResponse:
		return 44
	case *aioesphomeapi.ListEntitiesClimateResponse:
		return 46
	case *aioesphomeapi.ClimateStateResponse:
		return 47
	default:
		return 0
	}
}

func (c *conn) Hello(in *aioesphomeapi.HelloRequest) error {
	resp := aioesphomeapi.HelloResponse{
		ApiVersionMajor: 1,
		ApiVersionMinor: 3,
		ServerInfo:      "periphhome",
	}
	return c.reply(&resp)
}

func (c *conn) Connect(in *aioesphomeapi.ConnectRequest) error {
	resp := aioesphomeapi.ConnectResponse{
		InvalidPassword: c.n.cfg.API.Password != in.Password,
	}
	if err := c.reply(&resp); err != nil {
		return err
	}
	if resp.InvalidPassword {
		return errors.New("invalid password")
	}
	return nil
}

func (c *conn) Disconnect(in *aioesphomeapi.DisconnectRequest) error {
	if err := c.reply(&aioesphomeapi.DisconnectResponse{}); err != nil {
		return err
	}
	// Signal the caller that the connection has to be torn down if it hasn't
	// already.
	return io.EOF
}

func (c *conn) Ping(in *aioesphomeapi.PingRequest) error {
	return c.reply(&aioesphomeapi.PingResponse{})
}

func (c *conn) DeviceInfo(in *aioesphomeapi.DeviceInfoRequest) error {
	resp := aioesphomeapi.DeviceInfoResponse{
		UsesPassword:   c.n.cfg.API.Password != "",
		Name:           c.n.cfg.PeriphHome.Name,
		MacAddress:     c.n.mac,
		EsphomeVersion: "PeriphHome " + version,
		// TODO(maruel): Use -ldflags?
		// For now, pass Comment here.
		CompilationTime: c.n.cfg.PeriphHome.Comment,
		// We could probably add board name detection in periph.
		Model:        runtime.GOOS,
		HasDeepSleep: false,
	}
	return c.reply(&resp)
}

func (c *conn) ListEntities(in *aioesphomeapi.ListEntitiesRequest) error {
	for _, e := range c.n.entities {
		if err := c.reply(e.describe()); err != nil {
			return err
		}
	}
	return c.reply(&aioesphomeapi.ListEntitiesDoneResponse{})
}

func (c *conn) SubscribeStates(ctx context.Context, in *aioesphomeapi.SubscribeStatesRequest) error {
	// Interestingly, this means to subscribe to *all states*. There's no partial
	// subscription.
	for _, item := range c.n.entities {
		if item.getType() == cameraComponent {
			// Cameras are handled separately.
			continue
		}
		c.n.wg.Add(1)
		go func(cc component) {
			defer c.n.wg.Done()
			cc.subscribe(ctx, c)
		}(item)
	}
	return nil
}

func (c *conn) SubscribeLogs(in *aioesphomeapi.SubscribeLogsRequest) error {
	return errors.New("SubscribeLogs: not implemented")
}

func (c *conn) SubscribeHomeassistantServices(in *aioesphomeapi.SubscribeHomeassistantServicesRequest) error {
	// This is sent to notify the node that the client is HomeAssistant. For now,
	// just ignore.
	return nil
}

func (c *conn) SubscribeHomeAssistantStates(in *aioesphomeapi.SubscribeHomeAssistantStatesRequest) error {
	// For now, just ignore.
	/*
		if err := c.reply(&aioesphomeapi.SubscribeHomeAssistantStateResponse{
			EntityId: "yo",
		}); err != nil {
			return err
		}
	*/
	return nil
}

func (c *conn) OnHomeAssistantState(in *aioesphomeapi.HomeAssistantStateResponse) error {
	// I think we would only care if we have scripting.
	return nil
}

func (c *conn) GetTime(in *aioesphomeapi.GetTimeRequest) error {
	// YOLO: https://en.wikipedia.org/wiki/Year_2038_problem
	return c.reply(&aioesphomeapi.GetTimeResponse{
		EpochSeconds: uint32(time.Now().Unix()),
	})
}

func (c *conn) ExecuteService(in *aioesphomeapi.ExecuteServiceRequest) error {
	// TODO(maruel): Find via in.Key, then send command.
	return errors.New("ExecuteService: not implemented")
}

func (c *conn) CoverCommand(in *aioesphomeapi.CoverCommandRequest) error {
	e := c.n.lookup[in.Key]
	if e == nil {
		return fmt.Errorf("unknown item %x", in.Key)
	}
	return e.coverCommand(in)
}

func (c *conn) FanCommand(in *aioesphomeapi.FanCommandRequest) error {
	e := c.n.lookup[in.Key]
	if e == nil {
		return fmt.Errorf("unknown item %x", in.Key)
	}
	return e.fanCommand(in)
}

func (c *conn) LightCommand(in *aioesphomeapi.LightCommandRequest) error {
	e := c.n.lookup[in.Key]
	if e == nil {
		return fmt.Errorf("unknown item %x", in.Key)
	}
	return e.lightCommand(in)
}

func (c *conn) SwitchCommand(in *aioesphomeapi.SwitchCommandRequest) error {
	e := c.n.lookup[in.Key]
	if e == nil {
		return fmt.Errorf("unknown item %x", in.Key)
	}
	return e.switchCommand(in)
}

func (c *conn) CameraImage(ctx context.Context, in *aioesphomeapi.CameraImageRequest) error {
	// Warning: No Key is provided, which seems to imply only one camera can be
	// exported by device.
	// TODO(maruel): Confirm.
	for _, item := range c.n.entities {
		if item.getType() == cameraComponent {
			c.n.wg.Add(1)
			go func(cc component) {
				defer c.n.wg.Done()
				cc.cameraStream(ctx, c, in)
			}(item)
			return nil
		}
	}
	log.Printf("camera image requested but no camera is available")
	return nil
}

func (c *conn) ClimateCommand(in *aioesphomeapi.ClimateCommandRequest) error {
	e := c.n.lookup[in.Key]
	if e == nil {
		return fmt.Errorf("unknown item %x", in.Key)
	}
	return e.climateCommand(in)
}

// reply implements clientConn.
func (c *conn) reply(msg proto.Message) error {
	id := getID(msg)
	if id == 0 {
		return fmt.Errorf("internal error: implement ID for type %T", msg)
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	logf("reply(%T)", msg)
	if err := writeMsg(c.c, id, raw); err != nil {
		logf("failed to write")
		return err
	}
	return nil
}

//

// writeMsg writes one message.
func writeMsg(w io.Writer, id int, msg []byte) error {
	//logf("writeMsg(%d, %x)", id, msg)
	b := make([]byte, 1, 1+binary.MaxVarintLen32*2+len(msg))
	var buf [binary.MaxVarintLen32]byte
	n := binary.PutUvarint(buf[:], uint64(len(msg)))
	b = append(b, buf[:n]...)
	n = binary.PutUvarint(buf[:], uint64(id))
	b = append(b, buf[:n]...)
	b = append(b, msg...)
	_, err := w.Write(b)
	return err
}

// readMsg reads one message and returns it.
func readMsg(r io.Reader) (int, []byte, error) {
	//logf("readMsg: zero byte")
	var b [1]byte
	if _, err := r.Read(b[:]); err != nil {
		return 0, nil, err
	}
	if b[0] != 0 {
		return 0, nil, errors.New("expected byte zero")
	}
	//logf("readMsg: msgsize")
	msgsize, err := readVarUint(r)
	if err != nil {
		return 0, nil, err
	}
	if msgsize > 1024*1024 {
		return 0, nil, fmt.Errorf("msg size too large %d", msgsize)
	}
	//logf("readMsg: id; msgsize = %d", msgsize)
	id, err := readVarUint(r)
	if err != nil {
		return 0, nil, err
	}
	var msg []byte
	if msgsize != 0 {
		msg = make([]byte, msgsize)
		if _, err = r.Read(msg); err != nil {
			return 0, nil, err
		}
	}
	//logf("readMsg() id = %d, msg = %x", id, msg)
	return int(id), msg, err
}

// readVarUint is similar to binary.Uvarint() but reads one byte at a time.
func readVarUint(r io.Reader) (uint64, error) {
	var buf [1]byte
	var x uint64
	var s uint
	for i := 0; ; i++ {
		if _, err := r.Read(buf[:]); err != nil {
			return 0, err
		}
		b := buf[0]
		if b < 0x80 {
			if i >= binary.MaxVarintLen64 || i == binary.MaxVarintLen64-1 && b > 1 {
				return 0, errors.New("overflow")
			}
			return x | uint64(b)<<s, nil
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
}

// isErrEOF returns true if the error is functionally equivalent to io.EOF.
//
// This is needed because the error is different on Windows.
func isErrEOF(err error) bool {
	if err == io.EOF {
		logf("isErrEOF(%T %#v) = true", err, err)
		return true
	}
	if runtime.GOOS == "windows" {
		if oe, ok := err.(*net.OpError); ok && oe.Op == "read" {
			// Created by os.NewSyscallError()
			if se, ok := oe.Err.(*os.SyscallError); ok && se.Syscall == "wsarecv" {
				const WSAECONNABORTED = 10053
				const WSAECONNRESET = 10054
				switch n := se.Err.(type) {
				case syscall.Errno:
					v := n == WSAECONNRESET || n == WSAECONNABORTED
					logf("isErrEOF(%T %#v) = %t", se.Err, se.Err, v)
					return v
				default:
					logf("isErrEOF(%T %#v) = false", se.Err, se.Err)
					return false
				}
			}
		}
	}
	logf("isErrEOF(%T %#v) = false", err, err)
	return false
}

// shouldLog is set to true in unit test when testing.Verbose() is true.
var shouldLog = true

func logf(fmt string, v ...interface{}) {
	if shouldLog {
		log.Printf(fmt, v...)
	}
}
