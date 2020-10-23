package ratelimit

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/monmohan/rate-limiting/local"
)

var oneMinWindow = convertToMinuteWindow(1)
var inMemMapStore = &local.CounterStore{Counters: make(map[string]uint32)}

//timewindow is an interface describing operations on a generic window of time
//timewindow is represented as an integer indexed bucket. For example a one minute time window has 60 buckets from 0-59
// and 15 minute timewindow has 4 buckets from 0-3
type timewindow interface {
	current(ts time.Time) (cur int, percent float32)
	previous(cur int) (prev int)
}

//minutewindow is an implemention of TimeWindow for minute sized window
//Minimum size of such a window is 1 min whereas maximum is 30 to guarantee atleast two windows in an hour
type minutewindow struct {
	span int
}

//convertToMinuteWindow is a helper function which
//sets window to highest value <=30, that is a divisor of 60 e.g. if sz=17, actual window size will be 20
func convertToMinuteWindow(sz int) *minutewindow {
	if sz > 30 {
		sz = 30
	}

	buckets := 60 / sz
	n := 60 / buckets
	return &minutewindow{span: n}

}

func (mw *minutewindow) current(ts time.Time) (cur int, curPercent float32) {
	//Build the map keys
	curMin := ts.Minute()
	cur = curMin / mw.span
	minUsed := ts.Minute() % mw.span
	totalSecs := minUsed*60 + ts.Second()
	curPercent = float32(totalSecs) / float32((mw.span * 60))
	return

}

func (mw *minutewindow) previous(cur int) (prev int) {
	if cur == 0 {
		return (60 / mw.span) - 1
	}
	return cur - 1
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

//SlidingWindowRateLimiter is an implementation of Allower
type SlidingWindowRateLimiter struct {
	Limit           uint32
	Store           CounterStore
	ClientID        string
	Bucket          timewindow
	CurrentTimeFunc func() time.Time
}
type windowTime time.Time

func (f windowTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("H %d M %d S %d", time.Time(f).Hour(), time.Time(f).Minute(), time.Time(f).Second()))
}

// WindowStats is a struct holding information on window stats during processing
// Callers can examine that to make richer decisions, more than just allowing or denying the request
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

//PerMinute as name suggests provides "per minute"  based rate limiting
// Threhsold - the allowed rate, maximum requests in a minute
// id - a string identifying rate bucket.
// Generally it would be your userId or applicationID for which the rate bucket is created
// The default counter store is used which is an in-memory map. For production use Memcached backed store
func PerMinute(id string, threshold uint32) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, Bucket: oneMinWindow}
	return &s

}

//PerNMinute as name suggests provides rate limiting for minute window greater than 1
// 0 < N<= 30
// Threhsold - the allowed rate, maximum requests in a minute
// id - a string identifying rate bucket.
// Generally it would be your userId or applicationID for which the rate bucket is created
// The default counter store is used which is an in-memory map. For production use Memcached backed store
func PerNMinute(id string, threshold uint32, N int) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, Bucket: convertToMinuteWindow(N)}
	return &s

}

//Allow , throttle request if rpm exceeds threshold
func (w *SlidingWindowRateLimiter) Allow() bool {
	return w.AllowWithStats().Allow
}

//AllowWithStats - Return stats of the processed windows in addition to allow and deny
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
