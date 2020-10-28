package memcached

import (
	"fmt"
	"strconv"

	"github.com/bradfitz/gomemcache/memcache"
)

//CounterStore stores counters in Memcached servers
type CounterStore struct {
	Client *memcache.Client
}

func (mc CounterStore) String() string {
	return fmt.Sprintf("Memcache Store : Running at %v", mc.Client)
}

//Fetch returns the counters for the provided keys
func (mc *CounterStore) Fetch(prev string, cur string) (prevbucketcounter uint32, curbucketcounter uint32, err error) {
	result, err := mc.Client.GetMulti([]string{prev, cur})

	if err != nil && len(result) == 0 {
		return 0, 0, err
	}
	if v, ok := result[prev]; ok {
		val, err := strconv.ParseUint(string(v.Value), 10, 32)
		prevbucketcounter = uint32(val)
		if err != nil {
			fmt.Printf("Memcache Store : Invalid value for CounterID=%s! %v\n", prev, err.Error())
			prevbucketcounter = 0
		}
	}

	if v, ok := result[cur]; ok {
		val, err := strconv.ParseUint(string(v.Value), 10, 32)
		curbucketcounter = uint32(val)
		if err != nil {
			fmt.Printf("Memcache Store : Invalid value for CounterID=%s! %v\n", cur, err.Error())
			curbucketcounter = 0
		}
	}

	return prevbucketcounter, curbucketcounter, nil

}

func (mc *CounterStore) Incr(counterID string) error {
	_, err := mc.Client.Get(counterID)
	if err == memcache.ErrCacheMiss {
		//initialize, add a 5 minute expiration time
		addItem := memcache.Item{Key: counterID, Value: []byte("1"), Expiration: 300}
		mc.Client.Add(&addItem) //ignore error in case someone beat us to it
	} else {
		mc.Client.Increment(counterID, uint64(1))
	}
	return nil

}

func (mc CounterStore) Del(key string) error {
	err := mc.Client.Delete(key)
	if err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}
