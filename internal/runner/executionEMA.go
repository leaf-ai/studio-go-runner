package runner

import (
	"math"
	"sync"
	"time"
)

// TimeEMA is used to store exponential moving averages for a time duration
type TimeEMA struct {
	avgs map[time.Duration]time.Duration
	last time.Time
	sync.Mutex
}

// NewTimeEMA creates a new exponential moving average of a time duration
// for a set of time windows with an initial execution time duration set
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

// Update is used to update the moving average based on a new duration that
// has been observed
//
func (avgs *TimeEMA) Update(value time.Duration) {
	avgs.Lock()
	defer avgs.Unlock()

	fdtime := time.Since(avgs.last)
	avgs.last = time.Now()

	for ftime, oldValue := range avgs.avgs {
		alpha := 1.0 - math.Exp(-fdtime.Seconds()/ftime.Seconds())
		r := alpha*value.Seconds() + (1.0-alpha)*oldValue.Seconds()
		avgs.avgs[ftime] = time.Duration(time.Duration(r) * time.Second)
	}
}

// Keys can be used to retrieve a list of the moving average periods across which
// the average is calculated
func (avgs *TimeEMA) Keys() (keys []time.Duration) {
	avgs.Lock()
	defer avgs.Unlock()

	keys = make([]time.Duration, 0, len(avgs.avgs))
	for k := range avgs.avgs {
		keys = append(keys, k)
	}
	return keys
}

// Get retrieves a single time duration moving average for a specified window of time
func (avgs *TimeEMA) Get(window time.Duration) (avg time.Duration, wasPresent bool) {
	avgs.Lock()
	defer avgs.Unlock()

	avg, wasPresent = avgs.avgs[window]
	return avg, wasPresent
}
