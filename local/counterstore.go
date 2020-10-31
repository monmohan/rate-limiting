package local

import (
	"sync"
)

//CounterStore stores counters in a Map
//Used only for basic testing, can't be used for distributed case
//Use MemcachedStore instead
type CounterStore struct {
	mu       sync.Mutex //guard counters map
	Counters map[string]uint32
}

//Incr increments the counter value for the given key represented by counterID
func (im *CounterStore) Incr(counterID string) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.Counters[counterID] = im.Counters[counterID] + 1
	return nil
}

//Fetch returns the stored counter values for the given keys
//This is used by the sliding window rate limiter to get the current minute and previous minute counter values
func (im *CounterStore) Fetch(prev string, cur string) (PrevMinute uint32, current uint32, err error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.Counters[prev], im.Counters[cur], nil
}

//Del deletes the key and the stored counter value
func (im *CounterStore) Del(key string) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	delete(im.Counters, key)
	return nil
}
