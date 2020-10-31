package ratelimit

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/monmohan/rate-limiting/local"
)

var oneMinWindow = convertToMinuteWindow(1)
var oneSecWindow = convertToSecondWindow(1)
var inMemMapStore = &local.CounterStore{Counters: make(map[string]uint32)}

//CounterStore manages counter value for each window
type CounterStore interface {
	Incr(counterID string) error
	Fetch(prevWindowIdx string, curWindowIdx string) (prevWindowCounter uint32, curWindowCounter uint32, err error)
	Del(counterID string) error
}

//Allower interface, implementations would provide different algorithms
//to enforce ratelimits
type Allower interface {
	Allow() bool
}

//timewindow is an interface describing operations on a generic window of time
//timewindow is represented as an integer indexed window. For example a one minute time window has 60 windows from 0-59
// and 15 minute timewindow has 4 windows, indexed from 0 to 3
type timewindow interface {
	current(ts time.Time) (cur int, percent float32)
	previous(cur int) (prev int)
}

//minutewindow is an implemention of timeWindow for minute sized window
//Minimum size of such a window is 1 min whereas maximum is 30 to guarantee atleast two windows in an hour
type minutewindow struct {
	size int
}

//convertToMinuteWindow is a helper function which
//sets window to highest value <=30, that is a divisor of 60 e.g. if sz=17, actual window size will be 20
func convertToMinuteWindow(sz int) *minutewindow {
	return &minutewindow{size: calculateSize(sz, 60)}

}
func calculateSize(sz int, max int) int {
	if sz > max/2 {
		sz = max / 2
	}

	windows := max / sz
	return max / windows

}

func (mw *minutewindow) current(ts time.Time) (cur int, curPercent float32) {
	//Build the map keys
	curMin := ts.Minute()
	cur = curMin / mw.size
	minUsed := ts.Minute() % mw.size
	totalSecs := minUsed*60 + ts.Second()
	curPercent = float32(totalSecs) / float32((mw.size * 60))
	return

}

func (mw *minutewindow) previous(cur int) (prev int) {
	if cur == 0 {
		return (60 / mw.size) - 1
	}
	return cur - 1
}

type secondWindow struct {
	size int
}

func (sw *secondWindow) current(ts time.Time) (cur int, curPercent float32) {
	//Build the map keys
	curSecond := ts.Second()
	cur = curSecond / sw.size
	millis := (ts.Nanosecond() / (1000 * 1000))
	totalMillis := (curSecond%sw.size)*1000 + millis
	curPercent = float32(totalMillis) / float32((sw.size * 1000))
	fmt.Printf("current: total millis %d, percent %2f\n", totalMillis, curPercent)
	return

}

func (sw *secondWindow) previous(cur int) (prev int) {
	if cur == 0 {
		return (60 / sw.size) - 1
	}
	return cur - 1
}

func convertToSecondWindow(sz int) *secondWindow {
	return &secondWindow{size: calculateSize(sz, 60)}

}

//SlidingWindowRateLimiter is an implementation of Allower
type SlidingWindowRateLimiter struct {
	Limit           uint32
	Store           CounterStore
	ClientID        string
	windowInUse     timewindow
	CurrentTimeFunc func() time.Time
}
type windowTimeFormatter time.Time

func (f windowTimeFormatter) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("H %d M %d S %d MS %d",
		time.Time(f).Hour(),
		time.Time(f).Minute(),
		time.Time(f).Second(),
		(time.Time(f).Nanosecond() / (1000 * 1000)),
	))
}

// WindowStats is a struct holding information on window stats during processing
// Callers can examine that to make richer decisions, more than just allowing or denying the request
type WindowStats struct {
	RequestTime               time.Time `json:"-"`
	FormattedWindowTime       windowTimeFormatter
	CurrentWindowIndex        int
	CurrentCounter            uint32
	PreviousWindowIndex       int
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

// PerMinute as name suggests provides "per minute"  based rate limiting
// Threhsold - the allowed rate, maximum requests in a minute
// id - A string that identifies the what the rate applies to.
// Generally it would be your userId or applicationID for which this rate limiter is created
// By default, an in-memory counter store is used. For production use Memcached backed store
func PerMinute(id string, threshold uint32) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, windowInUse: oneMinWindow}
	return &s

}

// PerNMinute as name suggests provides rate limiting for minute window greater than 1
// 0 < N<= 30
// Threhsold - the allowed rate, maximum requests in a minute
// id - A string that identifies the what the rate applies to.
// Generally it would be your userId or applicationID for which this rate limiter is created
// By default, an in-memory counter store is used. For production use Memcached backed store
func PerNMinute(id string, threshold uint32, N int) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, windowInUse: convertToMinuteWindow(N)}
	return &s

}

// PerSecond as name suggests provides "per second"  based rate limiting
// Threhsold - the allowed rate, maximum requests in a minute
// id - A string that identifies the what the rate applies to.
// Generally it would be your userId or applicationID for which this rate limiter is created
// By default, an in-memory counter store is used. For production use Memcached backed store
func PerSecond(id string, threshold uint32) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, windowInUse: oneSecWindow}
	return &s

}

//PerNSecond as name suggests provides rate limiting for second window greater than 1
// 0 < N<= 30
// Threhsold - the allowed rate, maximum requests in a minute
// id - A string that identifies the what the rate applies to.
// Generally it would be your userId or applicationID for which this rate limiter is created
// By default, an in-memory counter store is used. For production use Memcached backed store
func PerNSecond(id string, threshold uint32, N int) *SlidingWindowRateLimiter {
	s := SlidingWindowRateLimiter{ClientID: id, Limit: threshold, Store: inMemMapStore, windowInUse: convertToSecondWindow(N)}
	return &s

}

//Allow , throttle request if allowing this request would mean that we would cross the threshold specified by the rate limiter
func (w *SlidingWindowRateLimiter) Allow() bool {
	return w.AllowWithStats().Allow
}

//AllowWithStats - Return stats of the processed windows in addition to allow and deny
func (w *SlidingWindowRateLimiter) AllowWithStats() (stats WindowStats) {
	now := time.Now()
	if w.CurrentTimeFunc != nil {
		now = w.CurrentTimeFunc()
	}
	stats = WindowStats{RequestTime: now, FormattedWindowTime: windowTimeFormatter(now)}
	curWindowIdx, curWinUsePerc := w.windowInUse.current(now)
	prevWindowIdx := w.windowInUse.previous(curWindowIdx)
	stats.PreviousWindowIndex, stats.CurrentWindowIndex = prevWindowIdx, curWindowIdx

	//key is ClientID#minute [0-59] and value is the counter
	storeKeyPrevMin := fmt.Sprintf("%s#%d", w.ClientID, prevWindowIdx)
	storeKeyCurMin := fmt.Sprintf("%s#%d", w.ClientID, curWindowIdx)
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
		w.Store.Incr(fmt.Sprintf("%s#%d", w.ClientID, curWindowIdx))
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
