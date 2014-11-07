package util

import (
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/quipo/statsd"
)

var (
	statsdClient = newStatsDClient("localhost:8125", "govuk_crawler_worker.")
)

func GetEnvDefault(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	return val
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

func StatsDTiming(label string, start, end time.Time) {
	statsdClient.Timing("time."+label,
		int64(end.Sub(start)/time.Millisecond))
}

func StatsDGauge(label string, value int64) {
	statsdClient.Gauge("gauge."+label, value)
}

func newStatsDClient(host, prefix string) *statsd.StatsdClient {
	statsdClient := statsd.NewStatsdClient(host, prefix)
	statsdClient.CreateSocket()

	return statsdClient
}
