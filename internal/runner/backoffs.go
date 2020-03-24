package runner

// This contains the implementation of a TTL cache that stores the timestamp of the intended absolute time
// of expiry as the value.

import (
	"time"

	ttlCache "github.com/karlmutch/go-cache"
)

type Backoffs struct {
	backoffs *ttlCache.Cache
}

var (
	backoffs = &Backoffs{backoffs: ttlCache.New(10*time.Second, time.Minute)}
)

func GetBackoffs() (backoffs *Backoffs) {
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
	return result.(time.Time), present
}
