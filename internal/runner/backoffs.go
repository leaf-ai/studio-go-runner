package runner

// This contains the implementation of a TTL cache that stores the timestamp of the intended absolute time
// of expiry as the value.

import (
	"sync"
	"time"

	ttlCache "github.com/karlmutch/go-cache"
)

type Backoffs struct {
	backoffs *ttlCache.Cache
}

var (
	singleGet   sync.Mutex
	backoffOnce sync.Once
	backoffs    *Backoffs
)

func GetBackoffs() (backoffs *Backoffs) {
	singleGet.Lock()
	defer singleGet.Unlock()

	backoffOnce.Do(
		func() {
			backoffs = &Backoffs{backoffs: ttlCache.New(10*time.Second, time.Minute)}
		})
	return backoffs
}

func (b *Backoffs) Set(k string, d time.Duration) {
	// Use the existing timer if there is one and find out which one is the
	// longest and use that
	if expires, isPresent := b.Get(k); isPresent && time.Now().Add(d).Before(expires) {
		return
	}
	// is the longest time from now and use that
	b.backoffs.Set(k, time.Now().Add(d), d)
}

func (b *Backoffs) Get(k string) (expires time.Time, isPresent bool) {
	result, present := b.backoffs.Get(k)
	if !present {
		return expires, present
	}
	return result.(time.Time), present
}
