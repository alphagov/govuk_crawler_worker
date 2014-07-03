package util

import (
	"io"
	"net"
	"os"
	"sync"
)

func GetEnvDefault(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	return val
}

type ReconnectMutex struct {
	mutex        sync.RWMutex
	reconnecting bool
}

func (r *ReconnectMutex) Check() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.reconnecting
}

func (r *ReconnectMutex) Update(state bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.reconnecting = state
}

// ProxyTCP is a basic TCP proxy which can terminate connections. It can be
// used to test reconnect behaviour.
type ProxyTCP struct {
	sync.Mutex
	listener net.Listener
	remote   string
	conns    []net.Conn
	wg       sync.WaitGroup
}

func NewProxyTCP(lAddr, rAddr string) (*ProxyTCP, error) {
	ln, err := net.Listen("tcp", lAddr)
	if err != nil {
		return nil, err
	}

	proxy := &ProxyTCP{
		listener: ln,
		remote:   rAddr,
	}
	go proxy.AcceptLoop()

	return proxy, nil
}

func (p *ProxyTCP) Addr() string {
	return p.listener.Addr().String()
}

func (p *ProxyTCP) AcceptLoop() {
	for {
		p.wg.Add(1)
		defer p.wg.Done()

		lConn, err := p.listener.Accept()
		if err != nil {
			return
		}

		p.Lock()
		p.conns = append(p.conns, lConn)
		p.Unlock()

		rConn, err := net.Dial("tcp", p.remote)
		if err != nil {
			return
		}

		go io.Copy(lConn, rConn)
		go io.Copy(rConn, lConn)
	}
}

func (p *ProxyTCP) Close() {
	p.listener.Close()
	p.wg.Wait()
	p.KillConnected()
}

func (p *ProxyTCP) KillConnected() {
	p.Lock()
	defer p.Unlock()
	for _, conn := range p.conns {
		conn.Close()
	}
}
