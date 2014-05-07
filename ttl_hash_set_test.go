package govuk_crawler_worker_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/fzzy/radix/redis"
)

var _ = Describe("TTLHashSet", func() {
	prefix := "govuk_mirror_crawler_test"

	It("returns an error when asking for a TTLHashSet object that can't connect to redis", func() {
		ttlHashSet, err := NewTTLHashSet(prefix, "127.0.0.1:20000")

		Expect(err).ToNot(BeNil())
		Expect(ttlHashSet).To(BeNil())
	})

	Describe("Working with a redis service", func() {
		var (
			ttlHashSet    *TTLHashSet
			ttlHashSetErr error
		)

		BeforeEach(func() {
			ttlHashSet, ttlHashSetErr = NewTTLHashSet(prefix, "127.0.0.1:6379")
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(purgeAllKeys(prefix, "127.0.0.1:6379"))
		})

		It("should connect successfully with no errors", func() {
			Expect(ttlHashSetErr).To(BeNil())
			Expect(ttlHashSet).NotTo(BeNil())
		})

		It("should return false when a key doesn't exist", func() {
			exists, err := ttlHashSet.Exists("foobar")

			Expect(err).To(BeNil())
			Expect(exists).To(Equal(false))
		})

		It("exposes a way of adding a key to redis", func() {
			key := "foo.bar.baz"
			added, addedErr := ttlHashSet.Add(key)

			Expect(addedErr).To(BeNil())
			Expect(added).To(Equal(true))

			exists, existsErr := ttlHashSet.Exists(key)

			Expect(existsErr).To(BeNil())
			Expect(exists).To(Equal(true))
		})

		Describe("TTL()", func() {
			It("should return a negative TTL on a non-existent key", func() {
				ttl, err := ttlHashSet.TTL("this.key.does.not.exist")

				Expect(err).To(BeNil())
				Expect(ttl).To(Equal(-2))
			})

			It("should expose a positive TTL on key that exists", func() {
				key := "some.ttl.key"
				added, addErr := ttlHashSet.Add(key)

				Expect(addErr).To(BeNil())
				Expect(added).To(Equal(true))

				ttl, err := ttlHashSet.TTL(key)

				Expect(err).To(BeNil())
				Expect(ttl).To(BeNumerically(">", 1000))
			})
		})
	})
})

func purgeAllKeys(prefix string, address string) error {
	client, err := redis.Dial("tcp", address)
	if err != nil {
		return err
	}

	keys, err := client.Cmd("KEYS", prefix+"*").List()
	if err != nil {
		return err
	}

	reply := client.Cmd("DEL", keys)
	if reply.Err != nil {
		return reply.Err
	}

	return nil
}
