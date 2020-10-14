package ratelimit

import (
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/monmohan/rate-limiting/local"
	"github.com/monmohan/rate-limiting/memcached"
)

type StoreMode string

const (
	INMEM     StoreMode = "ram"
	MEMCACHED StoreMode = "memcache"
)

var inmem *string

func init() {
	inmem = flag.String("mode", "ram", "Which store to use? memcache ram")

}

func getRateLimiter(threshold int) Allower {
	flag.Parse()
	if *inmem == string(INMEM) {
		fmt.Println("Tests running in-memory mode...")
		return NewSlidingWindow("TestClientSimple", RPMLimit(threshold), &local.CounterStore{Counters: make(map[string]uint32)})
	}
	mc := NewSlidingWindow("TestClientSimple", RPMLimit(threshold), configureMemcache())
	e := mc.Reset()
	if e != nil {
		panic(fmt.Sprintf("Counter Reset for Memcache failed Tests can't be executed, error=%s", e.Error()))
	}
	return mc
}

func TestSimpleSliding(t *testing.T) {
	fmt.Println("TestSimpleSliding ")
	threshold := 50
	err := 1
	w := getRateLimiter(threshold)

	numtimes := 0
	for {
		result := w.Allow()
		if !result {
			fmt.Printf("Number of times %d\n", numtimes)
			break

		}
		numtimes++
		if numtimes > (threshold + err) {
			t.Fatalf("Traffic wasn't throttled")
		}

	}

}

func TestBasicSliding(t *testing.T) {
	fmt.Println("TestBasicSliding ")
	threshold := 50
	w := getRateLimiter(threshold)
	var wg sync.WaitGroup
	err := 2 //allow for some laxity

	func1 := func(numReqToSend int, previous int) {
		defer wg.Done()
		fmt.Printf("Sending %d Requests in minute window = %d, Second := %d \n", numReqToSend, time.Now().Minute(), time.Now().Second())
		total, i := 0, 0
		for i < numReqToSend {
			result := w.Allow()
			total = previous + i

			if result && total > threshold+err {
				t.Fatalf("Throttling failed, Total requests sent = %d\n", total)
			}
			if !result {
				if total < threshold {
					t.Fatalf("Throttled unnecessarily, Total requests sent = %d\n", total)
				}
				fmt.Printf("Total Requests sent %d\n", total)
				break
			}
			i++
		}

	}

	secToNextMin := time.Second * time.Duration(60-time.Now().Second())
	wg.Add(1)
	time.AfterFunc(secToNextMin, func() {
		func1(20, 0)
	}) //send first call at start of the minute
	nextCall := secToNextMin + 40*time.Second
	wg.Add(1)
	time.AfterFunc(nextCall, func() {
		func1(40, 20)
	}) //send second call 40 seconds after first
	wg.Wait()

}

func TestSlidingMultiWindow(t *testing.T) {
	fmt.Println("TestSlidingMultiWindow ")
	threshold := 120
	w := getRateLimiter(threshold)
	curTimeMin, curTimeSec := time.Now().Minute(), time.Now().Second()
	secToNextMin := 60 - curTimeSec
	fmt.Printf("Current Min:= %d, Seconds until next Min:= %d\n", curTimeMin, secToNextMin)
	err := 2
	done := make(chan int)

	sendRequest := func(numRequests int, maxAllowed int) {
		fmt.Printf("Called at Time Min,Sec = %d,%d ; MaxAllowed=%d\n", time.Now().Minute(), time.Now().Second(), maxAllowed)
		i := 0
		defer func() { done <- i }()
		for ; i < numRequests; i++ {
			result := w.Allow()
			if result && i > (maxAllowed+err) {
				t.Fatalf("Throttling failed, Total requests sent = %d\n", i)
			}
			if !result {
				if i < (maxAllowed - err) {
					t.Fatalf("Throttled unnecessarily, Total requests sent = %d\n", i)
				}
				break
			}

		}

	}
	nextMaxAllowed := threshold
	nextReqFunc := func() { sendRequest(threshold, nextMaxAllowed) }
	time.AfterFunc(0*time.Second, nextReqFunc) //immediate
	fmt.Printf("Total requests %d\n", <-done)
	//since we make the call 10 seconds in 2nd window
	//10/60 of new window is used
	nextReqTime := time.Duration(int64(secToNextMin+10) * int64(time.Second))
	totalInWindow2 := 0
	nextMaxAllowed = int(float32(threshold) / 6)
	totalInWindow2 += nextMaxAllowed
	time.AfterFunc(nextReqTime, nextReqFunc)
	fmt.Printf("Total requests %d\n", <-done)
	//since we make the call in 30 seconds in 2nd window and have
	//already consumed 1/6 of threshold , ((40/60)*threshold)-(already consumed) is the value it can have
	nextMaxAllowed = int(float32(threshold*2)/float32(3)) - nextMaxAllowed
	totalInWindow2 += nextMaxAllowed
	time.AfterFunc(30*time.Second, nextReqFunc)
	fmt.Printf("Total requests now %d\n", <-done)

	nextMaxAllowed = threshold - int(float32(5*totalInWindow2)/float32(6))
	time.AfterFunc(30*time.Second, nextReqFunc)
	fmt.Printf("Total requests now %d\n", <-done)

}
func configureMemcache() CounterStore {
	c := memcache.New("127.0.0.1:11211")
	c.MaxIdleConns = 5
	return &memcached.CounterStore{Client: c}

}
