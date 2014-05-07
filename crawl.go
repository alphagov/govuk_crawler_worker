package govuk_crawler_worker

import (
	"errors"
	"io/ioutil"
	"net/http"
	"sync"
)

var (
	RetryRequestError error = errors.New("Retry request: 429 or 5XX HTTP Response returned")

	statusCodes []int
	once        sync.Once
)

func RetryStatusCodes() []int {
	// This is go's equivalent of memoization/macro expansion. It's
	// being used here because we have a fixed array we're generating
	// with known values.
	once.Do(func() {
		statusCodes = []int{429}

		for i := 500; i <= 599; i++ {
			statusCodes = append(statusCodes, i)
		}
	})

	return statusCodes
}

func Crawl(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}

	if contains(RetryStatusCodes(), resp.StatusCode) {
		return []byte{}, RetryRequestError
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return body, nil
}

func contains(haystack []int, needle int) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}

	return false
}
