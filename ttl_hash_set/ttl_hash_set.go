package ttl_hash_set

import (
	"sync"
	"time"

	"github.com/fzzy/radix/redis"
)

type TTLHashSet struct {
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

	return exists, err
}

func (t *TTLHashSet) TTL(key string) (int, error) {
	localKey := prefixKey(t.prefix, key)

	t.mutex.Lock()
	ttl, err := t.client.Cmd("TTL", localKey).Int()
	t.mutex.Unlock()

	return ttl, err
}

func prefixKey(prefix string, key string) string {
	return prefix + ":" + key
}
