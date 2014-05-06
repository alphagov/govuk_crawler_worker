package govuk_crawler_worker

import (
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
