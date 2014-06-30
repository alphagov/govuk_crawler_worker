package ttl_hash_set

import (
	"net"
	"sync"
	"time"

	"github.com/fzzy/radix/redis"
)

const WaitBetweenReconnect = 2 * time.Second

type TTLHashSet struct {
	addr   string
	client *redis.Client
	mutex  sync.Mutex
	prefix string
}

func NewTTLHashSet(prefix string, address string) (*TTLHashSet, error) {
	client, err := redis.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	return &TTLHashSet{
		addr:   address,
		client: client,
		prefix: prefix,
	}, nil
}

func (t *TTLHashSet) Add(key string) (bool, error) {
	localKey := prefixKey(t.prefix, key)

	// Use pipelining to set the key and set expiry in one go.
	t.mutex.Lock()
	t.client.Append("SET", localKey, 1)
	t.client.Append("EXPIRE", localKey, (12 * time.Hour).Seconds())
	add, err := t.client.GetReply().Bool()
	t.mutex.Unlock()

	if err != nil {
		t.reconnectIfIOError(err)
	}

	return add, err
}

func (t *TTLHashSet) Close() error {
	t.mutex.Lock()
	err := t.client.Close()
	t.mutex.Unlock()

	return err
}

func (t *TTLHashSet) Exists(key string) (bool, error) {
	localKey := prefixKey(t.prefix, key)

	t.mutex.Lock()
	exists, err := t.client.Cmd("EXISTS", localKey).Bool()
	t.mutex.Unlock()

	if err != nil {
		t.reconnectIfIOError(err)
	}

	return exists, err
}

// Sends a PING to the underlying Redis service. This can be used to
// healthcheck any Redis servers we're connected to.
func (t *TTLHashSet) Ping() (string, error) {
	t.mutex.Lock()
	ping, err := t.client.Cmd("PING").Str()
	t.mutex.Unlock()

	if err != nil {
		t.reconnectIfIOError(err)
	}

	return ping, err
}

// Reconnect initiates a new connection to the server. It will return
// immediately after locking the mutex, but other operations will be blocked
// until the reconnect is successful (preventing further errors and
// reconnects)
func (t *TTLHashSet) Reconnect() {
	t.mutex.Lock()

	go func() {
		for {
			client, err := redis.Dial("tcp", t.addr)
			if err == nil {
				t.client = client
				t.mutex.Unlock()
				return
			}

			time.Sleep(WaitBetweenReconnect)
		}
	}()
}

func (t *TTLHashSet) TTL(key string) (int, error) {
	localKey := prefixKey(t.prefix, key)

	t.mutex.Lock()
	ttl, err := t.client.Cmd("TTL", localKey).Int()
	t.mutex.Unlock()

	if err != nil {
		t.reconnectIfIOError(err)
	}

	return ttl, err
}

// Radix closes the connection if it encounters an error. By calling this on
// non-nil errors we can prevent subsequent queries from failing.
func (t *TTLHashSet) reconnectIfIOError(err error) {
	errStr := err.Error()
	_, netErr := err.(*net.OpError)

	if netErr || errStr == "EOF" || errStr == "use of closed network connection" {
		t.Reconnect()
	}
}

func prefixKey(prefix string, key string) string {
	return prefix + ":" + key
}
