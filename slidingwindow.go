package ratelimit

import (
	"fmt"
	"time"
)

type TimeWindow interface {
	current(ts time.Time) (cur int, percent float32)
	previous(cur int) (prev int)
}

type MinuteWindow struct {
	n int
}

/**
 * Set window to highest value <=30, that is a divisor of 60 e.g. if sz=17, actual window size will be 20
 */
func makeMinuteWindow(sz int) *MinuteWindow {
	if sz > 30 {
		sz = 30
	}

	buckets := 60 / sz
	n := 60 / buckets
	return &MinuteWindow{n: n}

}

func (mw *MinuteWindow) current(ts time.Time) (cur int, curPercent float32) {
	//Build the map keys
	curMin := ts.Minute()
	cur = curMin / mw.n
	minUsed := ts.Minute() % mw.n
	totalSecs := minUsed*60 + ts.Second()
	curPercent = float32(totalSecs) / float32((mw.n * 60))
	return

}

func (mw *MinuteWindow) previous(cur int) (prev int) {
	prev = cur - 1
	if prev < 0 {
		prev = (prev + mw.n) % mw.n
	}
	return
}

//RPMLimit defines the maximum number of events allowed in a minute
type RPMLimit uint32

//CounterStore manages counter value for each minute bucket
type CounterStore interface {
	Incr(counterID string) error
	Fetch(prev string, cur string) (PrevMinute uint32, current uint32, err error)
	Del(key string) error
}

//Allower interface, implementations would provide different algorithms
//to enforce ratelimits
type Allower interface {
	Allow() bool
}

//SlidingWindow is an implementation of RateLimiter
type SlidingWindow struct {
	rpmLimit RPMLimit
	store    CounterStore
	clientID string
	bucket   TimeWindow
}

func (w SlidingWindow) String() string {
	return fmt.Sprintf("CliendID = %s Threshold=%d, Store : %T", w.clientID, w.rpmLimit, w.store)

}

//NewSlidingWindow creates a rate limiter which implements Sliding Window counter
// Threhsold - the allowed rate, (only supports requests per minute)
// id - a string identifying rate bucket.
// Generally it would be your userId or applicationID for which the rate bucket is created
// CounterStore See CounterStore. For production usage MemcachedStore is recommended
func NewSlidingWindow(id string, threshold RPMLimit, counterStore CounterStore) *SlidingWindow {
	s := SlidingWindow{clientID: id, rpmLimit: threshold, store: counterStore, bucket: makeMinuteWindow(1)}
	return &s

}

//Allow , throttle request if rpm exceeds threshold
func (w *SlidingWindow) Allow() bool {
	curMin, usedWin2 := w.bucket.current(time.Now())
	prevMin := w.bucket.previous(curMin)

	//key is ClientID#minute [0-59] and value is the counter
	storeKeyPrevMin := fmt.Sprintf("%s#%d", w.clientID, prevMin)
	storeKeyCurMin := fmt.Sprintf("%s#%d", w.clientID, curMin)
	lastMinCounter, curMinCounter, err := w.store.Fetch(storeKeyPrevMin, storeKeyCurMin)
	if err != nil {
		fmt.Printf("Unable to fetch counters, ERR = %s, will allow requests\n ", err.Error())
		return true
	}
	//how much of the window have we used so far
	percWin1Used := 1 - usedWin2
	win1Used := uint32(percWin1Used * float32(lastMinCounter))
	rollingCtr := win1Used + curMinCounter
	//fmt.Printf("PercWin1Used %f, used Win1Used %d cur Ctr %d rolling Cter %d \n", percWin1Used, win1Used, curMinCounter, rollingCtr)
	if rollingCtr <= uint32(w.rpmLimit) {
		//	fmt.Printf("Incrementing: Min %d, Rolling Counter %d , Win1 %d, Current %d\n", curMin, rollingCtr, win1Used, curMinCounter)
		w.store.Incr(fmt.Sprintf("%s#%d", w.clientID, curMin))
		return true
	}
	fmt.Printf("Throttling: Min %d Rolling Counter %d , Win1 %d, Current %d\n", curMin, rollingCtr, win1Used, curMinCounter)
	return false
}

//Reset deletes all counter for the client
func (w *SlidingWindow) Reset() error {
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("%s#%d", w.clientID, i)
		e := w.store.Del(key)
		if e != nil {
			return e
		}

	}
	return nil

}
