package ttl_hash_set

import (
	"net"
	"sync"
	"time"

	"github.com/fzzy/radix/redis"
)

const WaitBetweenReconnect = 2 * time.Second
const ttlExpiryTime = 48 * time.Hour

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

type TTLHashSet struct {
	addr    string
	client  *redis.Client
	mutex   sync.Mutex
	prefix  string
	rcMutex ReconnectMutex
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
	t.client.Append("EXPIRE", localKey, ttlExpiryTime.Seconds())
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

// Reconnect asynchronously initiates a new connection to the server if
// there's not already one in progress. Other operations will continue to
// return errors until this has succeeded.
func (t *TTLHashSet) Reconnect() {
	if t.rcMutex.Check() {
		return
	}

	t.rcMutex.Update(true)
	go func() {
		defer t.rcMutex.Update(false)

		for {
			client, err := redis.Dial("tcp", t.addr)
			if err == nil {
				t.mutex.Lock()
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
