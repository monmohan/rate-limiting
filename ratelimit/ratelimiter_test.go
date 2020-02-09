package ratelimit

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
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

func getRateLimiter(threshold int) *RateLimiter {
	flag.Parse()
	if *inmem == string(INMEM) {
		return NewRateLimiter("TestClientSimple", threshold, &InMemoryStore{counters: make(map[string]int)})
	}
	mc := NewRateLimiter("TestClientSimple", threshold, configureMemcache())
	n, e := mc.ResetCounters()
	if e != nil {
		panic(fmt.Sprintf("Counter Reset for Memcache failed Tests can't be executed, counters reset =%d, error=%s", n, e.Error()))
	}
	return mc
}

func TestSimpleSliding(t *testing.T) {
	fmt.Println("TestSimpleSliding ")
	w := getRateLimiter(50)

	numtimes := 0
	for {
		result := w.AllowRequest()
		if !result {
			fmt.Printf("Number of times %d\n", numtimes)
			break

		}
		numtimes++
		if numtimes > (w.threshold + 1) {
			t.Fatalf("Traffic wasn't throttled")
		}

	}

}

func TestBasicSliding(t *testing.T) {
	fmt.Println("TestBasicSliding ")
	threshold := 50
	w := getRateLimiter(threshold)
	done := make(chan int)
	var func2 func()
	totalRequest := 0
	err := 2 //allow for some laxity

	func1 := func() {
		fmt.Printf("Sending 20 Requests in minute window = %d\n", time.Now().Minute())
		for i := 0; i < 20; i++ {
			result := w.AllowRequest()
			if !result && totalRequest < threshold {
				t.Fatalf("Throttled unnecessarily !! %d\n", i)
			}
			totalRequest++
		}
		time.AfterFunc(40*time.Second, func2)
	}

	func2 = func() {
		fmt.Printf("Sending another 40 Requests in minute window = %d", time.Now().Minute())
		defer func() { done <- totalRequest }()
		for i := 0; i < 40; i++ {
			result := w.AllowRequest()
			if result && totalRequest > (threshold+err) {
				t.Fatalf("Throttling failed, Total requests sent = %d\n", totalRequest)
			}
			if !result {
				if totalRequest < threshold {
					t.Fatalf("Throttled unnecessarily, Total requests sent = %d\n", totalRequest)
				}
				break
			}
			totalRequest++
		}

	}

	time.AfterFunc(10*time.Second, func1)

	totalRequest = <-done //wait for the goroutines to finish
	fmt.Printf("Total Requests Sent = %d\n", totalRequest)

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
			result := w.AllowRequest()
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
func configureMemcache() *MemcachedStore {
	return &MemcachedStore{Client: memcache.New("127.0.0.1:11211")}

}
