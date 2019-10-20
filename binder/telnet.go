package binder

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/meklis/http-snmpwalk-proxy/logger"
	"github.com/ztrue/tracerr"
	"io"
	"net"
	"reflect"
	"time"
	"unsafe"
)

type (
	Binder struct {
		Error         error
		ClientTimeout time.Duration
		DeviceTimeout time.Duration
		lg            *logger.Logger
		client        *net.Conn
		device        *net.Conn
		controller    chan Signal
	}
	BinderConfig struct {
		DeviceTimeout time.Duration
		ClientTimeout time.Duration
		Logger        *logger.Logger
	}
	Signal struct {
		Error   error
		Message string
	}
)

func InitBinder(conf BinderConfig) *Binder {
	binder := new(Binder)
	binder.lg = conf.Logger
	binder.DeviceTimeout = conf.DeviceTimeout
	binder.ClientTimeout = conf.ClientTimeout

	binder.controller = make(chan Signal, 10)
	return binder
}

func (c *Binder) BindChannel(client net.Conn, device net.Conn) *Binder {
	// conn -> telnet
	go func() {
		b := make([]byte, 1)
		for {
			_, err := client.Read(b)
			if err == io.EOF {
				c._closeBinding("EOF signal from client")
				return
			} else if err != nil {
				c._criticalErr("error read from client: %v", tracerr.Wrap(err))
				return
			}
			if bytes.Contains([]byte{0x00, 0xFF, 0x02, 0x01, 0x03, 0x04, 0x07, 0x08}, b) {
				continue
			}
			_, err = device.Write(b)
			if err != nil {
				c._criticalErr("error writing to conn: %v", tracerr.Wrap(err))
				return
			}
			device.SetDeadline(time.Now().Add(c.DeviceTimeout))
			client.SetDeadline(time.Now().Add(c.ClientTimeout))
		}
	}()

	//telnet -> conn
	go func() {
		b := make([]byte, 1)
		for {
			_, err := device.Read(b)

			if err == io.EOF {
				c._closeBinding("EOF signal from telnet")
				return
			} else if err != nil {
				c._criticalErr("error read from telnet: %v", tracerr.Wrap(err))
				return
			}
			_, err = client.Write(b)
			if err != nil {
				c._criticalErr("error writing to conn: %v", tracerr.Wrap(err))
				return
			}
			device.SetDeadline(time.Now().Add(c.DeviceTimeout))
			client.SetDeadline(time.Now().Add(c.ClientTimeout))
		}
	}()

	return c
}

func (c *Binder) BindChannelStream(client net.Conn, device net.Conn) *Binder {
	// conn -> telnet
	go func() {
		for {
			writer := bufio.NewWriter(device)
			_, err := writer.ReadFrom(client)
			if err == io.EOF {
				c._closeBinding("connection is closed by foreign host")
			} else if err != nil {
				c._criticalErr("error write to telnet - %v", err.Error())
			} else {
				c._criticalErr("unknown error when writing to telnet")
			}
			return
		}
	}()

	//telnet -> conn
	go func() {
		for {
			writer := bufio.NewWriter(client)
			_, err := writer.ReadFrom(device)
			if err == io.EOF {
				c._closeBinding("connection is closed by foreign host")
			} else if err != nil {
				c._criticalErr("error write to client - %v", err.Error())
			} else {
				c._criticalErr("unknown error when writing to client")
			}
			return
		}
	}()

	return c
}
func (c *Binder) CloseBinder() {
	close(c.controller)
	c = nil
}

func (c *Binder) Wait() (error, string) {
	for {
		time.Sleep(time.Millisecond * 100)
		if len(c.controller) >= 1 {
			resp := <-c.controller
			return resp.Error, resp.Message
		}
	}
}

func (c *Binder) _criticalErr(message string, args ...interface{}) {
	c.lg.DebugF(message, args...)
	if !isChanClosed(c.controller) {
		c.controller <- Signal{
			Error: fmt.Errorf(message, args...),
		}
	}
}
func (c *Binder) _closeBinding(message string, args ...interface{}) {
	c.lg.DebugF(message, args...)
	if !isChanClosed(c.controller) {
		c.controller <- Signal{
			Message: fmt.Sprintf(message, args...),
		}
	}
}

func isChanClosed(ch interface{}) bool {
	if reflect.TypeOf(ch).Kind() != reflect.Chan {
		panic("only channels!")
	}

	// get interface value pointer, from cgo_export
	// typedef struct { void *t; void *v; } GoInterface;
	// then get channel real pointer
	cptr := *(*uintptr)(unsafe.Pointer(
		unsafe.Pointer(uintptr(unsafe.Pointer(&ch)) + unsafe.Sizeof(uint(0))),
	))

	// this function will return true if chan.closed > 0
	// see hchan on https://github.com/golang/go/blob/master/src/runtime/chan.go
	// type hchan struct {
	// qcount   uint           // total data in the queue
	// dataqsiz uint           // size of the circular queue
	// buf      unsafe.Pointer // points to an array of dataqsiz elements
	// elemsize uint16
	// closed   uint32
	// **

	cptr += unsafe.Sizeof(uint(0)) * 2
	cptr += unsafe.Sizeof(unsafe.Pointer(uintptr(0)))
	cptr += unsafe.Sizeof(uint16(0))
	return *(*uint32)(unsafe.Pointer(cptr)) > 0
}
