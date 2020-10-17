package ratelimit

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/monmohan/rate-limiting/local"
)

var defaultWindow = ConvertToTimeWindow(1)
var defaultStore = &local.CounterStore{Counters: make(map[string]uint32)}

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
func ConvertToTimeWindow(sz int) *MinuteWindow {
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

//SlidingWindowRateLimiter is an implementation of RateLimiter
type SlidingWindowRateLimiter struct {
	Limit           uint32
	Store           CounterStore
	ClientID        string
	Bucket          TimeWindow
	CurrentTimeFunc func() time.Time
}
type windowTime time.Time

func (f windowTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("H %d M %d S %d", time.Time(f).Hour(), time.Time(f).Minute(), time.Time(f).Second()))
}

type WindowStats struct {
	RequestTime               time.Time `json:"-"`
	WindowTime                windowTime
	CurrentBucketID           int
	CurrentCounter            uint32
	PreviousBucketID          int
	PreviousWindowUsedPercent float32
	PreviousWindowUseCount    uint32
	RollingCounter            uint32
	Allow                     bool
	LastErr                   error `json:",omitempty"`
}

func (ws WindowStats) String() string {
	b, err := json.Marshal(ws)
	if err != nil {
		fmt.Println(err.Error())
	}
	return string(b)
}

func (w SlidingWindowRateLimiter) String() string {
	return fmt.Sprintf("CliendID = %s Threshold=%d, Store : %T", w.ClientID, w.Limit, w.Store)

}

//NewRpmLimiter as name suggests provides "per minute"  based rate limiting
// Threhsold - the allowed rate, maximum requests in a minute
// id - a string identifying rate bucket.
// Generally it would be your userId or applicationID for which the rate bucket is created
// The default counter store is used which is an in-memory map. For production use Memcached backed store
func NewRpmLimiter(id string, threshold uint32) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: defaultStore, Bucket: defaultWindow}
	return &s

}

//Allow , throttle request if rpm exceeds threshold
func (w *SlidingWindowRateLimiter) Allow() bool {
	stats := w.AllowWithStats()
	fmt.Println(stats)
	return stats.Allow

}

func (w *SlidingWindowRateLimiter) AllowWithStats() (stats WindowStats) {
	now := time.Now()
	if w.CurrentTimeFunc != nil {
		now = w.CurrentTimeFunc()
	}
	stats = WindowStats{RequestTime: now, WindowTime: windowTime(now)}
	curMin, curWinUsePerc := w.Bucket.current(now)
	prevMin := w.Bucket.previous(curMin)
	stats.PreviousBucketID, stats.CurrentBucketID = prevMin, curMin

	//key is ClientID#minute [0-59] and value is the counter
	storeKeyPrevMin := fmt.Sprintf("%s#%d", w.ClientID, prevMin)
	storeKeyCurMin := fmt.Sprintf("%s#%d", w.ClientID, curMin)
	lastMinCounter, curMinCounter, err := w.Store.Fetch(storeKeyPrevMin, storeKeyCurMin)
	if err != nil {
		stats.LastErr = fmt.Errorf("Unable to fetch counters, ERR = %s, will allow requests\n ", err.Error())
		stats.Allow = true
		return

	}
	//how much of the window have we used so far
	prevWinUsePerc := 1 - curWinUsePerc
	prevWinUseCount := uint32(prevWinUsePerc * float32(lastMinCounter))
	rollingCtr := prevWinUseCount + curMinCounter
	stats.PreviousWindowUsedPercent, stats.PreviousWindowUseCount, stats.RollingCounter, stats.CurrentCounter = prevWinUsePerc, prevWinUseCount, rollingCtr, curMinCounter

	if rollingCtr <= uint32(w.Limit) {
		//fmt.Printf("Incrementing: Min %d, Rolling Counter %d , Win1 %d, Current %d\n", curMin, rollingCtr, prevWinUseCount, curMinCounter)
		w.Store.Incr(fmt.Sprintf("%s#%d", w.ClientID, curMin))
		stats.Allow = true
		return
	}
	stats.Allow = false
	return
}

//Reset deletes all counter for the client
func (w *SlidingWindowRateLimiter) Reset() error {
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("%s#%d", w.ClientID, i)
		e := w.Store.Del(key)
		if e != nil {
			return e
		}

	}
	return nil

}
