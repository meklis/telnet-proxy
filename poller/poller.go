package poller

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type (
	Poller struct {
		sync.RWMutex
		binds                 map[string]Bind
		MaxConnections        int
		MaxConnectionsPerHost int
	}
	Bind struct {
		Key     string `json:"key"`
		Client  string `json:"client"`
		Device  string `json:"device"`
		StartAt int64  `json:"start_at"`
	}
)

func Init(max_conns, max_conns_per_host int) *Poller {
	poll := new(Poller)
	poll.MaxConnectionsPerHost = max_conns_per_host
	poll.MaxConnections = max_conns
	poll.binds = make(map[string]Bind)
	return poll
}

func (c *Poller) GetConnList() []Bind {
	binds := make([]Bind, 0)
	for _, b := range c.binds {
		binds = append(binds, b)
	}
	return binds
}

func (c *Poller) AddBind(bind Bind) {
	c.Lock()
	defer c.Unlock()
	key := fmt.Sprintf("%v-%v", bind.Client, bind.Device)
	bind.StartAt = time.Now().Unix()
	bind.Key = key
	c.binds[key] = bind
	return
}
func (c *Poller) DeleteBind(bind Bind) {
	c.Lock()
	defer c.Unlock()
	key := fmt.Sprintf("%v-%v", bind.Client, bind.Device)
	delete(c.binds, key)
	return
}
func (c *Poller) ResetBindStat() {
	c.Lock()
	defer c.Unlock()
	c.binds = make(map[string]Bind)
}
func (c *Poller) IsConnectAllowed(ip string) bool {
	c.Lock()
	defer c.Unlock()

	if len(c.binds) > c.MaxConnections {
		return false
	}
	iterator := 0
	for _, bind := range c.binds {
		splitted := strings.Split(bind.Device, ":")
		if len(splitted) > 0 {
			if splitted[0] == ip {
				iterator++
			}
		}
		if iterator >= c.MaxConnectionsPerHost {
			return false
		}
	}
	return true
}
