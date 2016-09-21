// Package healthcheck provides the ability to run and report on checks
// designed to give an overview of the basic health of a system.
//
// Individual checks are implementations of the healthcheck.Checker interface,
// for example:
//
//   type okChecker struct { }
//   func (okChecker) Name() string { return "my_check" }
//   func (okChecker) Check() (healthcheck.StatusEnum, error) {
//     return healthcheck.OK, nil
//   }
//
// The easiest way to create a healthcheck is to use the
// healthcheck.NewHealthCheck function which will return a healthcheck value
// with all given checks configured and a default check timeout applied.
//
//   hc := healthcheck.NewHealthCheck(okChecker{})
//
// All checks can then be run by calling Status() on the healthcheck by calling
// the Check() method of each.  All individual checks are run in parallel, and
// each subsequent call to Status() will re-run all checks.
//
//   status := hc.Status()
//
// Check Timeouts
//
// If you use NewHealthCheck to create a HealthCheck value, it will have a
// default check timeout of 1 second applied.  If this timeout expires before a
// check returns, that check will be deemed to have failed and will be given a
// `critical` status.  The same timeout length applies to each individual
// check.
//
// A you require a custom timeout length, you can either set the timeout after
// calling NewHealthCheck, or create a HealthCheck value manually.
//
//   timeout := time.Minute
//   hc := healthcheck.NewHealthCheck(...)
//   hc.timeout = timeout
//   // or
//   hc := HealthCheck{Checkers: checkers, Timeout: timeout)}
//
// If you don't want a timeout to be applied, set the value to a zero or
// negative duration.
//
// HTTPHandler Func
//
// HealthCheck.HTTPHandler is provided as a mechanism to easily expose the
// results of your healthcheck by using the result via http.HandleFunc:
//
//   hc := healthcheck.NewHealthCheck(...)
//   http.HandleFunc("/healthcheck", hc.HTTPHandler())
//   log.Fatal(http.ListenAndServe(":12345", nil))
//
// The resulting output will be JSON-encoded and include both an overall
// system health status, as well as the results of any individual checks.
//
//   {
//     "status": "critical",
//     "checks": {
//       "check_1": {
//         "status": "ok"
//       },
//       "check_2": {
//         "status": "warning",
//         "message": "Extra info about the warning"
//       },
//       "check_3": {
//         "status": "critical",
//         "message": "Extra info about the critical state"
//       }
//     }
//   }
package healthcheck

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// StatusEnum is a enum type for the valid states of a service.
type StatusEnum int

const (
	// OK is the status of a service when it is operating correctly.
	OK StatusEnum = iota
	// Warning is the status of a service when it is still available but some
	// systems may be unavailable or in an incorrect state.
	Warning
	// Critical is the status of a service in a critical state.
	Critical
)

// statusValues is a mapping of StatusEnum values to their human-readable form.
var statusValues = map[StatusEnum]string{
	OK:       "ok",
	Warning:  "warning",
	Critical: "critical",
}

// String converts a numeric status identifer to a human-readable
// representation, or returns `unknown` if the status can't be identified.
func (s StatusEnum) String() string {
	if status, ok := statusValues[s]; ok {
		return status
	}
	return "unknown"
}

// MarshalJSON is a concrete implementation of the json.Marshaler interface
// which allows StatusEnum values to be converted to their human-readable
// representation when serialising to JSON.
func (s StatusEnum) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// Checker is the interface which all individual status checks must satisfy.
type Checker interface {
	Name() string
	Check() (StatusEnum, error)
}

// Status represents the overall status of the application.
type Status struct {
	Status StatusEnum       `json:"status"`
	Checks map[string]Check `json:"checks,omitEmpty"`
	mutex  sync.RWMutex
}

// NewStatus returns a new Status value with a default status state of `ok`
// and an empty set of Checks.
func NewStatus() Status {
	return Status{Status: OK, Checks: map[string]Check{}}
}

// AddCheckResult adds a Check to the Status in a concurrent-access-safe
// manner.  It also updates the overal s.Status value if the new Check's status
// value is higher (e.g. if the current status is `ok` and the new Check's
// status is `warning` or `critical`).
func (s *Status) AddCheckResult(name string, check Check) {
	s.mutex.Lock()
	s.Checks[name] = check
	if s.Status < check.Status {
		s.Status = check.Status
	}
	s.mutex.Unlock()
}

// Check is a single check which is performed as part of the overall system
// status.  All individual check statuses must be `ok` for the overall system
// status to be `ok`.
type Check struct {
	Status  StatusEnum `json:"status"`
	Message string     `json:"message,omitempty"`
}

// HealthCheck encapsulates and performs checks which are used to identify the
// health of a the application.  `Timeout` specifies the duration to wait until
// an individual check is deemed to have failed.  If this value is zero or
// negative, no timeout is applied.
type HealthCheck struct {
	Timeout  time.Duration
	Checkers []Checker
}

// DefaultCheckTimeout is the default time period used by NewHealthCheck, after
// which checks will be deemed to have failed.
const DefaultCheckTimeout = time.Second

// NewHealthCheck is a helper function for quickly creating a new HealthCheck
// value with the appropriate checkers in place and a default timeout.
func NewHealthCheck(checkers ...Checker) *HealthCheck {
	return &HealthCheck{Checkers: checkers, Timeout: DefaultCheckTimeout}
}

// Status runs all checks and responds with the individual statuses for those
// checks, as well as an overall status.
//
// If all checks are `ok`, then the overall status will also be `ok`.  If one
// or more checks are in a `warning` state, and no checks are in a `critical`
// state, then the overall status will be `warning`.  If one or more checks are
// in a `critical` state, the overall state will be `critical`.
//
// If any check fails to return within a `HealthCheck.Timeout` duration then
// the check will be deemed to have failed.  In this situation, the individual
// check status will be set to `critical` and an appropraite message will be
// added.  If the value of h.Timeout is zero or negative then no timeout is
// applied and a check may block forever.
func (h *HealthCheck) Status() *Status {
	status := NewStatus()

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = math.MaxInt64
	}

	var wg sync.WaitGroup
	wg.Add(len(h.Checkers))

	for _, checker := range h.Checkers {
		go func(checker Checker) {
			defer wg.Done()
			result := make(chan Check)
			chk := Check{}

			go func() {
				c := Check{}

				var err error
				c.Status, err = checker.Check()

				if err != nil {
					c.Message = err.Error()
				}

				result <- c
			}()

			select {
			case c := <-result:
				chk = c
			case <-time.After(timeout):
				chk = Check{
					Status:  Critical,
					Message: "Check timed out",
				}
			}

			status.AddCheckResult(checker.Name(), chk)
		}(checker)
	}

	wg.Wait()
	return &status
}

// HTTPHandler is a handler function for serving up the application healthcheck
// status via HTTP.
func (h *HealthCheck) HTTPHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		status := h.Status()
		encoder := json.NewEncoder(w)

		err := encoder.Encode(status)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot encode response data: %v", err),
				http.StatusInternalServerError)
		}
	}
}
