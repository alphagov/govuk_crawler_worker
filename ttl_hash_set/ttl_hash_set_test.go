package ttl_hash_set_test

import (
	. "github.com/alphagov/govuk_crawler_worker/ttl_hash_set"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"time"

	"github.com/alphagov/govuk_crawler_worker/util"
	// FIXME: Use the parent library once #35 has been fixed here:
	// https://github.com/fzzy/radix/issues/35
	"github.com/alphagov/radix/redis"
)

var _ = Describe("TTLHashSet", func() {
	redisAddr := util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
	prefix := "govuk_mirror_crawler_test"

	It("returns an error when asking for a TTLHashSet object that can't connect to redis", func() {
		ttlHashSet, err := NewTTLHashSet(prefix, "127.0.0.1:20000", time.Hour)

		Expect(err).ToNot(BeNil())
		Expect(ttlHashSet).To(BeNil())
	})

	Describe("Reconnects", func() {
		var (
			proxy         *util.ProxyTCP
			proxyAddr     string = "127.0.0.1:6380"
			key           string = "reconnect"
			ttlHashSet    *TTLHashSet
			reconnectTime time.Duration = 2 * time.Second
			delayBetween  time.Duration = reconnectTime / 10
		)

		BeforeEach(func() {
			var err error
			proxy, err = util.NewProxyTCP(proxyAddr, redisAddr)

			Expect(err).To(BeNil())
			Expect(proxy).ToNot(BeNil())

			ttlHashSet, err = NewTTLHashSet(prefix, proxyAddr, time.Hour)

			Expect(err).To(BeNil())
			Expect(ttlHashSet).ToNot(BeNil())
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(purgeAllKeys(prefix, redisAddr))
			proxy.Close()
		})

		It("should recover from connection errors", func() {
			_ = ttlHashSet.Incr(key)

			proxy.KillConnected()
			exists, err := ttlHashSet.Exists(key)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(MatchRegexp("EOF|connection reset by peer"))
			Expect(exists).To(Equal(false))

			Eventually(func() (bool, error) {
				return ttlHashSet.Exists(key)
			}).Should(Equal(true))
		})

		It("should return errors until reconnected", func() {
			_ = ttlHashSet.Incr(key)
			proxy.Close()

			start := time.Now()
			exists, err := ttlHashSet.Exists(key)

			Expect(err.Error()).To(MatchRegexp("EOF|connection reset by peer"))
			Expect(exists).To(Equal(false))

			time.Sleep(delayBetween) // Allow first reconnect to fail.
			proxy, err = util.NewProxyTCP(proxyAddr, redisAddr)

			Expect(err).To(BeNil())
			Expect(proxy).ToNot(BeNil())

			errorCount := 0
			for time.Since(start) < reconnectTime {
				exists, err := ttlHashSet.Exists(key)

				Expect(err).To(MatchError("use of closed network connection"))
				Expect(exists).To(Equal(false))

				time.Sleep(delayBetween)
				errorCount++
			}

			// Subtract one for the error and sleep before we restart ProxyTCP.
			expectedErrors := int((reconnectTime / delayBetween) - 1)
			Expect(errorCount).To(BeNumerically("~", expectedErrors, 2))

			exists, err = ttlHashSet.Exists(key)

			Expect(err).To(BeNil())
			Expect(exists).To(Equal(true))
		})
	})

	Describe("Working with a redis service", func() {
		var (
			ttlHashSet    *TTLHashSet
			ttlHashSetErr error
		)

		BeforeEach(func() {
			ttlHashSet, ttlHashSetErr = NewTTLHashSet(prefix, redisAddr, time.Hour)
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(purgeAllKeys(prefix, redisAddr))
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

		It("increments a key sequentially", func() {
			key := "foo.bar.baz"
			for i := 1; i < 5; i++ {
				incrErr := ttlHashSet.Incr(key)

				Expect(incrErr).To(BeNil())

				val, getErr := ttlHashSet.Get(key)

				Expect(getErr).To(BeNil())
				Expect(val).To(Equal(i))
			}
		})

		It("exposes a way to ping the underlying redis service", func() {
			ping, err := ttlHashSet.Ping()

			Expect(err).To(BeNil())
			Expect(ping).To(Equal("PONG"))
		})

		Describe("TTL()", func() {
			It("should return a negative TTL on a non-existent key", func() {
				ttl, err := ttlHashSet.TTL("this.key.does.not.exist")

				Expect(err).To(BeNil())
				Expect(ttl).To(BeNumerically("<", 0))
			})

			It("should expose a positive TTL on key that exists", func() {
				key := "some.ttl.key"
				addErr := ttlHashSet.Incr(key)

				Expect(addErr).To(BeNil())

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
