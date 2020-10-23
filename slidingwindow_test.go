package ratelimit

import (
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/bradfitz/gomemcache/memcache"
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

func getRateLimiter(threshold uint32) Allower {
	flag.Parse()
	mc := PerMinute("TestClientSimple", threshold)
	if *inmem == string(MEMCACHED) {
		memcacheStore := configureMemcache()
		fmt.Println("Tests running with memcache store...")
		mc.Store = memcacheStore
	}
	e := mc.Reset()
	if e != nil {
		panic(fmt.Sprintf("Counter Reset for Memcache failed Tests can't be executed, error=%s", e.Error()))
	}
	return mc
}

func getMutliMinRateLimiter(threshold uint32, minsz int, mockClock clock.Clock) *SlidingWindowRateLimiter {
	flag.Parse()
	mc := PerNMinute("TestMultiMin", threshold, minsz)
	if mockClock != nil {
		mc.CurrentTimeFunc = func() time.Time {
			return mockClock.Now()
		}
	}
	if *inmem == string(MEMCACHED) {
		memcacheStore := configureMemcache()
		fmt.Println("Tests running with memcache store...")
		mc.Store = memcacheStore
	}
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
	w := getMutliMinRateLimiter(uint32(threshold), 1, nil)

	numtimes := 0
	for {
		stats := w.AllowWithStats()
		fmt.Println(stats)
		result := stats.Allow

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
	w := getRateLimiter(uint32(threshold))
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
	fmt.Printf("Waiting for %v seconds to start the test..\n", secToNextMin)
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
	w := getRateLimiter(uint32(threshold))
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

func TestBasicSlidingMultiMinute(t *testing.T) {
	fmt.Println("TestBasicSlidingMultiMinute ")
	threshold := 50
	windowSizes := []int{10, 5, 15, 20, 12, 30}
	var wg sync.WaitGroup
	sendRequests := func(numReqToSend int, previous int, w *SlidingWindowRateLimiter, clock clock.Clock) {
		err := 2 //allow for some laxity
		defer wg.Done()
		fmt.Printf("Sending %d Requests in minute window = %d, Second := %d \n", numReqToSend, clock.Now().Minute(), clock.Now().Second())
		total, i := 0, 0
		for i < numReqToSend {
			stats := w.AllowWithStats()
			fmt.Println(stats)
			result := stats.Allow
			total = previous + i

			if result && total > threshold+err {
				t.Fatalf("Throttling failed, Total requests sent = %d\n", total)
			}
			if !result {
				if total < threshold {
					t.Fatalf("Throttled unnecessarily, Total requests sent = %d\n", total)
				}
				fmt.Printf("Throttled correctly, Total Requests sent %d\n", total)
				break
			}
			i++
		}

	}

	for _, winsz := range windowSizes {
		clock := clock.NewMock() // initialized to unix zero ts
		w := getMutliMinRateLimiter(uint32(threshold), winsz, clock)
		wg.Add(1)
		sendRequests(threshold/2, 0, w, clock)
		clock.Add(time.Duration(winsz/2) * time.Minute)
		wg.Add(1)
		sendRequests(threshold/2, threshold/2, w, clock)
		wg.Add(1)
		sendRequests(threshold/2, threshold, w, clock) //should fail
		wg.Wait()
	}

}

func TestSlidingMultiWindowMultiMin(t *testing.T) {
	fmt.Println("TestSlidingMultiWindowMultiMin ")
	threshold := 60
	windowSizes := []int{5, 10, 12, 15, 20, 30}
	//windowSizes := []int{30}
	err := 3
	sendRequest := func(numRequests int, maxAllowed int, w *SlidingWindowRateLimiter, clock clock.Clock) int {
		fmt.Printf("Called at Time Min,Sec = %d,%d ; MaxAllowed=%d\n", clock.Now().Minute(), clock.Now().Second(), maxAllowed)
		i := 0
		for ; i < numRequests; i++ {
			stats := w.AllowWithStats()
			fmt.Println(stats)
			result := stats.Allow
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
		fmt.Printf("Total Requests sent =%d\n", i)
		return i
	}

	for _, winsz := range windowSizes {
		mockClock := clock.NewMock() // initialized to unix zero ts
		w := getMutliMinRateLimiter(uint32(threshold), winsz, mockClock)
		durationInStartWin := mockClock.Now().Minute() % winsz
		nextMaxAllowed := threshold
		sendRequest(threshold/2, nextMaxAllowed, w, mockClock)
		mockClock.Add(1 * time.Minute) //add a minute and send rest
		sendRequest(threshold/2, nextMaxAllowed, w, mockClock)

		//advance clock by window size and 100 seconds into second window
		mockClock.Add(time.Duration(winsz) * time.Minute)
		mockClock.Add(40 * time.Second)
		nextMaxAllowed = int(float32(100+(60*durationInStartWin)) / float32((60 * winsz)) * float32(threshold))
		sendRequest(threshold, nextMaxAllowed, w, mockClock)
		//try again in 20 seconds, it should throttle
		mockClock.Add(20 * time.Second)
		s := sendRequest(threshold, 1, w, mockClock)
		mockClock.Add(1 * time.Minute)
		//3 minute into the second window by now
		nextMaxAllowed = (int(float32(180+(60*durationInStartWin)) / float32((winsz * 60)) * float32(threshold))) - (nextMaxAllowed + s)
		sendRequest(threshold, nextMaxAllowed, w, mockClock)
	}

}

func configureMemcache() CounterStore {
	c := memcache.New("127.0.0.1:11211")
	c.MaxIdleConns = 5
	return &memcached.CounterStore{Client: c}

}
