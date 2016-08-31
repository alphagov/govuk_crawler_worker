# Airbrake "legacy" Hook for Logrus <img src="http://i.imgur.com/hTeVwmJ.png" width="40" height="40" alt=":walrus:" class="emoji" title=":walrus:" />&nbsp;[![Build Status](https://travis-ci.org/gemnasium/logrus-airbrake-legacy-hook.svg?branch=master)](https://travis-ci.org/gemnasium/logrus-airbrake-legacy-hook)&nbsp;[![godoc reference](https://godoc.org/github.com/gemnasium/logrus-airbrake-legacy-hook?status.png)](https://godoc.org/gopkg.in/gemnasium/logrus-airbrake-legacy-hook.v1)

Use this hook to send your errors to [Airbrake](https://airbrake.io/).
This hook is using [`airbrake-go`](https://github.com/tobi/airbrake-go) behind the scenes.
The hook is a blocking call for `log.Error`, `log.Fatal` and `log.Panic`.

All logrus fields will be sent as context fields on Airbrake.

## Usage

The hook must be configured with:

* The URL of the api (ex: your [errbit](https://github.com/errbit/errbit) host url)
* An API key ID
* The name of the current environment ("development", "staging", "production", ...)

```go
import (
    "log/syslog"
    "github.com/Sirupsen/logrus"
    "gopkg.in/gemnasium/logrus-airbrake-legacy-hook.v1" // the package is named "aibrake"
    )

func main() {
    log := logrus.New()

    // Use the Airbrake hook to report errors that have Error severity or above to
    // an exception tracker. You can create custom hooks, see the Hooks section.
    log.AddHook(airbrake.NewHook("https://example.com", "xyz", "development"))
    log.Error("some logging message") // The error is sent to airbrake in background
}
```


