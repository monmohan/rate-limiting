package ds

import (
	"fmt"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

func TestSimpleSliding(t *testing.T) {
	inMem := NewRateLimiter("TestClientSimple", 50, &InMemoryStore{counters: make(map[string]int)})
	runTest(inMem, t)
	memcache := NewRateLimiter("TestClientSimple", 50, configureMemcache())
	runTest(memcache, t)

}
func runTest(w *RateLimiter, t *testing.T) {
	//call 51 times
	numtimes := 0
	for {
		result := w.AllowRequest()
		if !result {
			fmt.Printf("Number of times %d\n", numtimes)
			break

		}
		numtimes++
		if numtimes > 51 {
			t.Fatalf("Traffic wasn't throttled")
		}

	}
}

func TestBasicSliding(t *testing.T) {
	w := NewRateLimiter("TestClientBasicSliding", 50, configureMemcache())

	func1 := func() {
		for i := 0; i < 20; i++ {
			result := w.AllowRequest()
			if !result {
				t.Fatalf("Throttled unnecessarily !! %d\n", i)
				break

			}
		}
	}
	func2 := func() {
		for i := 0; i < 40; i++ {
			result := w.AllowRequest()
			if !result {
				fmt.Printf("Throttled !! %d\n", i)
				break

			}
			if i > 32 {
				t.Fatalf("Throttling failed !! %d\n", i)
			}
		}
	}

	time.AfterFunc(10*time.Second, func1)
	time.AfterFunc(40*time.Second, func2)

}

func TestSlidingMultiWindow(t *testing.T) {
	w := NewRateLimiter("TestClientSlidingMultiWindow", 50, &InMemoryStore{counters: make(map[string]int)})

	callsWithinWindow1 := func() {
		fmt.Println("Called in 40 sec")
		for i := 0; i < 50; i++ {
			result := w.AllowRequest()
			if !result {
				t.Fatalf("Throttled unnecessarily !! %d\n", i)
				break

			}
		}
	}
	time.AfterFunc(40*time.Second, callsWithinWindow1)

	/** Now We schedule a call after window expiry and that should work**/

	afterWindow2 := func() {
		fmt.Println("Called in 125 sec")
		for i := 0; i < 30; i++ {
			result := w.AllowRequest()
			if !result {
				t.Fatalf("Throttled unnecessarily !! %d\n", i)
				break

			}
		}
		fmt.Printf("Done, window state = %v  \n", *w)
	}

	callsInWindow2 := func() {
		fmt.Println("Called in 80 sec")
		for i := 0; i < 40; i++ {
			result := w.AllowRequest()
			if !result {
				fmt.Printf("Throttled, window state = %v\n, Num= %d\n", *w, i)
				break

			}

		}
	}
	time.AfterFunc(80*time.Second, callsInWindow2)
	time.AfterFunc(125*time.Second, afterWindow2)

	<-time.After(3 * time.Minute)

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
