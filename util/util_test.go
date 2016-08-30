package util_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"

	. "github.com/alphagov/govuk_crawler_worker/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Util", func() {
	Describe("ProxyTCP", func() {
		const statusCode = http.StatusNoContent
		var (
			proxy        *ProxyTCP
			remoteServer *httptest.Server
			localURL     string
		)

		BeforeEach(func() {
			remoteServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))
			remoteURL, _ := url.Parse(remoteServer.URL)

			var err error
			proxy, err = NewProxyTCP("127.0.0.1:0", remoteURL.Host)

			Expect(err).To(BeNil())
			Expect(proxy).ToNot(BeNil())

			localURL = fmt.Sprintf("http://%s", proxy.Addr())
		})

		AfterEach(func() {
			remoteServer.Close()
			proxy.Close()
		})

		It("should proxy connections", func() {
			resp, err := http.Get(localURL)

			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(statusCode))
		})

		It("should kill existing connections", func() {
			doGet := func() string {
				resp, err := http.Get(localURL)

				Expect(err).To(BeNil())
				Expect(resp.StatusCode).To(Equal(statusCode))

				conns := proxy.Connections()
				return fmt.Sprintf("%+v", (conns)[len(conns)-1])
			}

			// Check that we;re using the same connection initially.
			c1 := doGet()
			c2 := doGet()
			Expect(c1).To(Equal(c2))

			// Killing the connection should result in a new one.
			proxy.KillConnected()
			c3 := doGet()
			Expect(c2).NotTo(Equal(c3))
		})

		It("should be stoppable", func() {
			proxy.Close()
			resp, err := http.Get(localURL)

			urlErr, _ := err.(*url.Error)
			netErr, _ := urlErr.Err.(*net.OpError)

			Expect(netErr.Err.(*os.SyscallError).Err).To(Equal(syscall.ECONNREFUSED))
			Expect(resp).To(BeNil())
		})
	})

	Describe("GetEnvDefault", func() {
		It("will return the default value if no environment variable is set", func() {
			Expect(GetEnvDefault("SOME_NON_EXISTENT_ENV_VAR", "foo")).To(Equal("foo"))
		})

		It("will return the environment variable value if it's set", func() {
			env := "SOME_CUSTOM_CRAWLER_UTIL_ENV_VAR"
			os.Setenv(env, "200")

			Expect(GetEnvDefault(env, "foo")).To(Equal("200"))

			os.Setenv(env, "")
		})
	})
})
