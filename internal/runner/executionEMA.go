package runner

import (
	"math"
	"sync"
	"time"
)

// fdtime is time period for a execution time sampling
// ftime shows how fast it reacts on changes.
//
func (avgs *TimeEMA) Update(value time.Duration) {
	avgs.Lock()
	defer avgs.Unlock()

	fdtime := time.Now().Sub(avgs.last)
	avgs.last = time.Now()

	for ftime, oldValue := range avgs.avgs {
		alpha := 1.0 - math.Exp(-fdtime.Seconds()/ftime.Seconds())
		r := alpha*value.Seconds() + (1.0-alpha)*oldValue.Seconds()
		avgs.avgs[ftime] = time.Duration(time.Duration(r) * time.Second)
	}
}

func (avgs *TimeEMA) Keys() (keys []time.Duration) {
	avgs.Lock()
	defer avgs.Unlock()

	keys = make([]time.Duration, 0, len(avgs.avgs))
	for k := range avgs.avgs {
		keys = append(keys, k)
	}
	return keys
}

func (avgs *TimeEMA) Get(window time.Duration) (avg time.Duration, wasPresent bool) {
	avgs.Lock()
	defer avgs.Unlock()

	avg, wasPresent = avgs.avgs[window]
	return avg, wasPresent
}

func NewTimeEMA(windows []time.Duration, initial time.Duration) (emas *TimeEMA) {
	emas = &TimeEMA{
		avgs: make(map[time.Duration]time.Duration, len(windows)),
		last: time.Now(),
	}
	for _, window := range windows {
		emas.avgs[window] = initial
	}

	return emas
}

type TimeEMA struct {
	avgs map[time.Duration]time.Duration
	last time.Time
	sync.Mutex
}
