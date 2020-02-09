package ratelimit

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

type CounterStore interface {
	incr(counterID string) error
	fetch(prev string, cur string) (PrevMinute int, current int, err error)
	del(key string) error
}

type RateLimiter struct {
	threshold int
	store     CounterStore
	ClientID  string
}

func (w RateLimiter) String() string {
	return fmt.Sprintf("CliendID = %s threshold=%d, Store : %T", w.ClientID, w.threshold, w.store)

}
func NewRateLimiter(id string, threshold int, counterStore CounterStore) *RateLimiter {
	s := RateLimiter{ClientID: id, threshold: threshold, store: counterStore}
	return &s

}

//AllowRequest , throttle request if rpm exceeds threshold
func (w *RateLimiter) AllowRequest() bool {
	reqTS := time.Now()
	//Build the map keys
	curMin := reqTS.Minute()
	prevMin := curMin - 1
	if prevMin == -1 {
		prevMin = 59
	}
	//key is ClientID#minute [0-59] and value is the counter
	storeKeyPrevMin := fmt.Sprintf("%s#%d", w.ClientID, prevMin)
	storeKeyCurMin := fmt.Sprintf("%s#%d", w.ClientID, curMin)
	lastMinCounter, curMinCounter, err := w.store.fetch(storeKeyPrevMin, storeKeyCurMin)
	if err != nil {
		fmt.Printf("Unable to fetch counters, ERR = %s, will allow requests\n ", err.Error())
		return true
	}
	//how much of the window have we used so far
	secs := reqTS.Second()
	usedWin2 := float32(secs) / 60
	percWin1Used := 1 - usedWin2
	win1Used := int(percWin1Used * float32(lastMinCounter))
	rollingCtr := win1Used + curMinCounter
	//fmt.Printf("PercWin1Used %f, used Win1Used %d cur Ctr %d rolling Cter %d \n", percWin1Used, win1Used, curMinCounter, rollingCtr)
	if rollingCtr <= w.threshold {
		//fmt.Printf("Incrementing: Min %d, Rolling Counter %d , Win1 %d, Current %d\n", curMin, rollingCtr, win1Used, curMinCounter)
		w.store.incr(fmt.Sprintf("%s#%d", w.ClientID, curMin))
		return true
	}
	fmt.Printf("Throttling: Min %d Rolling Counter %d , Win1 %d, Current %d\n", curMin, rollingCtr, win1Used, curMinCounter)
	return false
}

//ResetCounters deletes all counter for the client
func (w *RateLimiter) ResetCounters() (succeeded int, err error) {
	completed := 0
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("%s#%d", w.ClientID, i)
		e := w.store.del(key)
		if e != nil {
			return completed, e
		}
		completed++
	}
	return completed, nil

}

//InMemoryStore stores counters in a Map
//Used only for basic testing, can't be used for distributed case
//Use MemcachedStore instead
type InMemoryStore struct {
	counters map[string]int
}

func (im InMemoryStore) String() string {
	return fmt.Sprintf("InMemory Store : Counters Map %v", im.counters)
}
func (im *InMemoryStore) incr(counterID string) error {
	im.counters[counterID] = im.counters[counterID] + 1
	return nil
}
func (im *InMemoryStore) fetch(prev string, cur string) (PrevMinute int, current int, err error) {
	return im.counters[prev], im.counters[cur], nil
}

func (im *InMemoryStore) del(key string) error {
	delete(im.counters, key)
	return nil
}

//MemcachedStore stores counters in Memcached servers
type MemcachedStore struct {
	Client *memcache.Client
}

func (mc MemcachedStore) String() string {
	return fmt.Sprintf("Memcache Store : Running at %v", mc.Client)
}

func (mc *MemcachedStore) fetch(prev string, cur string) (PrevMinute int, Current int, err error) {
	result, err := mc.Client.GetMulti([]string{prev, cur})

	if err != nil && len(result) == 0 {
		return 0, 0, fmt.Errorf("Memcache Store : Error in fetching item %v", err.Error())
	}
	if v, ok := result[prev]; ok {
		PrevMinute, err = strconv.Atoi(string(v.Value))
		if err != nil {
			fmt.Printf("Memcache Store : Invalid value for CounterID=%s! %v\n", prev, err.Error())
			PrevMinute = 0
		}
	}

	if v, ok := result[cur]; ok {
		Current, err = strconv.Atoi(string(v.Value))
		if err != nil {
			fmt.Printf("Memcache Store : Invalid value for CounterID=%s! %v\n", cur, err.Error())
			Current = 0
		}
	}

	return PrevMinute, Current, nil

}

func (mc *MemcachedStore) incr(counterID string) error {
	_, err := mc.Client.Get(counterID)
	if err == memcache.ErrCacheMiss {
		//initialize
		addItem := memcache.Item{Key: counterID, Value: []byte("1")}
		mc.Client.Add(&addItem) //ignore error in case someone beat us to it
	} else {
		mc.Client.Increment(counterID, uint64(1))
	}
	return nil

}

func (mc MemcachedStore) del(key string) error {
	err := mc.Client.Delete(key)
	if err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}
