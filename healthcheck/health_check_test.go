package healthcheck_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphagov/govuk_crawler_worker/healthcheck"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HealthCheck", func() {
	It("gives an OK status with zero checks", func() {
		hc := healthcheck.NewHealthCheck()
		Expect(hc.Status().Status).To(Equal(healthcheck.OK))
	})

	It("gives an OK status with multiple checks that are all OK", func() {
		hc := healthcheck.NewHealthCheck(okChecker{}, okChecker{}, okChecker{})
		Expect(hc.Status().Status).To(Equal(healthcheck.OK))
	})

	It("gives a Warning status with one check that is a Warning", func() {
		hc := healthcheck.NewHealthCheck(warningChecker{})
		Expect(hc.Status().Status).To(Equal(healthcheck.Warning))
	})

	It("gives a Warning status when at least one check is a Warning", func() {
		hc := healthcheck.NewHealthCheck(okChecker{}, warningChecker{}, okChecker{})
		Expect(hc.Status().Status).To(Equal(healthcheck.Warning))
	})

	It("gives a Critical status with one check that is Critical", func() {
		hc := healthcheck.NewHealthCheck(criticalChecker{})
		Expect(hc.Status().Status).To(Equal(healthcheck.Critical))
	})

	It("gives a Critical status when at least one check is Critical", func() {
		hc := healthcheck.NewHealthCheck(warningChecker{}, criticalChecker{}, okChecker{})
		Expect(hc.Status().Status).To(Equal(healthcheck.Critical))
	})

	It("gives correct individual statuses for each check", func() {
		hc := healthcheck.NewHealthCheck(okChecker{}, warningChecker{}, criticalChecker{})
		checks := hc.Status().Checks

		Expect(checks["ok"].Status).To(Equal(healthcheck.OK))
		Expect(checks["warning"].Status).To(Equal(healthcheck.Warning))
		Expect(checks["critical"].Status).To(Equal(healthcheck.Critical))
	})

	It("has the correct message for each check", func() {
		hc := healthcheck.NewHealthCheck(okChecker{}, warningChecker{}, criticalChecker{})
		checks := hc.Status().Checks

		Expect(checks["ok"].Message).To(Equal(""))
		Expect(checks["warning"].Message).To(Equal("A warning"))
		Expect(checks["critical"].Message).To(Equal("A critical failure"))
	})

	It("provides an HTTP handler function", func() {
		hc := healthcheck.NewHealthCheck(okChecker{})
		w := httptest.NewRecorder()
		hc.HTTPHandler()(w, nil)
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	It("correctly marshalls to JSON", func() {
		hc := healthcheck.NewHealthCheck(okChecker{}, criticalChecker{})
		w := httptest.NewRecorder()
		hc.HTTPHandler()(w, nil)
		Expect(strings.TrimSpace(w.Body.String())).To(Equal(`{"status":"critical","checks":{"critical":{"status":"critical","message":"A critical failure"},"ok":{"status":"ok"}}}`))
	})
})

func TestHealthCheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Health Check Suite")
}

// A checker that already returns `ok`
type okChecker struct{}

func (okChecker) Name() string { return "ok" }
func (okChecker) Check() (healthcheck.StatusEnum, error) {
	return healthcheck.OK, nil
}

// A checker that already returns `warning`
type warningChecker struct{}

func (warningChecker) Name() string { return "warning" }
func (warningChecker) Check() (healthcheck.StatusEnum, error) {
	return healthcheck.Warning, errors.New("A warning")
}

// A checker that already returns `critical`
type criticalChecker struct{}

func (criticalChecker) Name() string { return "critical" }
func (criticalChecker) Check() (healthcheck.StatusEnum, error) {
	return healthcheck.Critical, errors.New("A critical failure")
}
