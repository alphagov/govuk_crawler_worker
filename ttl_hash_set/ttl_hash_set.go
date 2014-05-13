package ttl_hash_set

import (
	"time"

	"github.com/fzzy/radix/redis"
)

type TTLHashSet struct {
	client *redis.Client
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
	t.client.Append("SET", localKey, 1)
	t.client.Append("EXPIRE", localKey, (24 * time.Hour).Seconds())

	return t.client.GetReply().Bool()
}

func (t *TTLHashSet) Close() error {
	return t.client.Close()
}

func (t *TTLHashSet) Exists(key string) (bool, error) {
	localKey := prefixKey(t.prefix, key)
	return t.client.Cmd("EXISTS", localKey).Bool()
}

func (t *TTLHashSet) TTL(key string) (int, error) {
	localKey := prefixKey(t.prefix, key)
	return t.client.Cmd("TTL", localKey).Int()
}

func prefixKey(prefix string, key string) string {
	return prefix + ":" + key
}
