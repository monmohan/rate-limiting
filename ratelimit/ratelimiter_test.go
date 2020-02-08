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
	return NewRateLimiter("TestClientSimple", threshold, configureMemcache())
}

func TestSimpleSliding(t *testing.T) {
	w := getRateLimiter(50)
	fmt.Printf("Using RateLimiter %s\n", *w)

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
	//already consumed 16 , ((40/60)*threshold)-16 is the value it can have
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

/*func TestSimpleMemcached(t *testing.T) {
	c := configureMemcache()
	item := memcache.Item{
		Key:   "foo",
		Value: []byte("1"),
	}

	//Byte array int
	buf := new(bytes.Buffer)
	var v int32 = 11
	err := binary.Write(buf, binary.LittleEndian, v)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
	}
	item2 := memcache.Item{
		Key:   "foo2",
		Value: buf.Bytes(),
	}
	err = c.Add(&item)
	if err != nil {
		t.Fatalf("Failed %s", err.Error())
	}
	n, err := c.Increment("foo", uint64(5))
	if err != nil {
		t.Fatalf("Failed incr %s", err.Error())
	}
	fmt.Printf("n=%d\n", n)

	err = c.Add(&item2)
	if err != nil {
		t.Fatalf("Failed 2 %s\n", err.Error())
	}
	nb, err := c.Increment("foo2", uint64(5))
	if err != nil {
		t.Fatalf("Failed incr %s\n", err.Error())
	}
	fmt.Printf("nb=%d\n", nb)
}*/
