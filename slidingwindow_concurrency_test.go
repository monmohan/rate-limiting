package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
)

func TestConcurrentCallers(t *testing.T) {
	fmt.Println("TestConcurrentCallers")
	threshold := 100
	winsizes := []int{1, 5, 20, 30}
	for _, winsz := range winsizes {
		w := getMutliMinRateLimiter(uint32(threshold), winsz, nil)
		numConcurCallers := 3
		done := make(chan int, numConcurCallers)
		err := 2 * numConcurCallers
		maxCalls := 2 * threshold //just to avoid deadlock

		sendRequest := func() {
			i := 0
			for i < maxCalls {
				result := w.Allow()

				if !result {
					fmt.Printf("Throttled %d\n", i)
					done <- i
					break
				}
				i++
			}
		}
		for i := 0; i < numConcurCallers; i++ {
			go sendRequest()
		}
		reqSent := 0
		for i := 0; i < 3; i++ {
			reqSent += <-done
		}

		fmt.Printf("Total Requests Sent %d\n", reqSent)
		if reqSent > (threshold + err) {
			t.Fatalf("Failed to throttle correctly, %d requests were sent", reqSent)
		}
	}

}

func TestConcurrentSlidingMultiWindow(t *testing.T) {
	fmt.Printf("Start Test: %s \n", t.Name())
	threshold := 120
	w := getRateLimiter(uint32(threshold))
	curTimeMin, curTimeSec := time.Now().Minute(), time.Now().Second()
	secToNextMin := 60 - curTimeSec
	fmt.Printf("Current Min:= %d, Seconds until next Min:= %d\n", curTimeMin, secToNextMin)
	numConcurCallers := 3
	err := 2 * numConcurCallers
	done := make(chan int, numConcurCallers)

	sendRequest := func() {
		fmt.Printf("Called at Time Min,Sec = %d,%d ;\n", time.Now().Minute(), time.Now().Second())
		i := 0
		for ; i < 2*threshold; i++ {
			result := w.Allow()
			if !result {
				fmt.Printf("Throttled %d\n", i)
				done <- i
				break
			}
		}

	}

	sendParallel := func(delay time.Duration, requestMaker func()) {
		for i := 0; i < numConcurCallers; i++ {
			go func() {
				time.AfterFunc(delay, requestMaker)
			}()
		}
	}
	validate := func(allowed int) {
		fmt.Printf("Sending concurrent requests, Maxallowed %d\n", allowed)
		reqSent := 0
		for i := 0; i < 3; i++ {
			reqSent += <-done
		}

		fmt.Printf("Total Requests Sent %d\n", reqSent)
		if reqSent > (allowed + err) {
			t.Fatalf("Failed to throttle correctly, %d requests were sent", reqSent)
		}
	}

	nextMaxAllowed := threshold
	sendParallel(0*time.Second, sendRequest)
	validate(nextMaxAllowed)

	//since we make the call 10 seconds in 2nd window
	//10/60 of new window is used
	nextReqTime := time.Duration(int64(secToNextMin+10) * int64(time.Second))
	totalInWindow2 := 0
	nextMaxAllowed = int(float32(threshold) / 6)
	totalInWindow2 += nextMaxAllowed

	sendParallel(nextReqTime, sendRequest)
	validate(nextMaxAllowed)

	//since we make the call in 30 seconds in 2nd window and have
	//already consumed 1/6 of threshold , ((40/60)*threshold)-(already consumed) is the value it can have
	nextMaxAllowed = int(float32(threshold*2)/float32(3)) - nextMaxAllowed
	totalInWindow2 += nextMaxAllowed
	sendParallel(30*time.Second, sendRequest)
	validate(nextMaxAllowed)

	nextMaxAllowed = threshold - int(float32(5*totalInWindow2)/float32(6))
	sendParallel(30*time.Second, sendRequest)
	validate(nextMaxAllowed)

}

func TestConcurrentSlidingMultiWindowMultiMin(t *testing.T) {
	fmt.Println("TestConcurrentSliding-MultiWindow-MultiMin ")
	threshold := 300
	windowSizes := []int{5, 10, 12, 15, 20, 30}
	//windowSizes := []int{30}
	numConcurCallers := 4
	err := 2 * numConcurCallers
	done := make(chan int, numConcurCallers)

	sendRequest := func(w *SlidingWindowRateLimiter, clock clock.Clock) {
		fmt.Printf("Called at Time Min,Sec = %d,%d ;\n", clock.Now().Minute(), clock.Now().Second())
		i := 0
		for ; i < 2*threshold; i++ {
			stats := w.AllowWithStats()
			//fmt.Println(stats)
			if !stats.Allow {
				fmt.Printf("Throttled %d\n", i)
				done <- i
				break
			}
		}

	}

	sendParallel := func(w *SlidingWindowRateLimiter, c clock.Clock) {
		for i := 0; i < numConcurCallers; i++ {
			go sendRequest(w, c)
		}
	}
	validate := func(allowed int) {
		fmt.Printf("Sending concurrent requests, Maxallowed %d\n", allowed)
		reqSent := 0
		for i := 0; i < numConcurCallers; i++ {
			reqSent += <-done
		}

		fmt.Printf("Total Requests Sent %d\n", reqSent)
		if reqSent > (allowed + err) {
			t.Fatalf("Failed to throttle correctly, %d requests were sent", reqSent)
		}
	}

	for _, winsz := range windowSizes {
		mockClock := clock.NewMock() // initialized to unix zero ts
		w := getMutliMinRateLimiter(uint32(threshold), winsz, mockClock)
		durationInStartWin := mockClock.Now().Minute() % winsz
		nextMaxAllowed := threshold
		sendParallel(w, mockClock)
		validate(nextMaxAllowed)

		//advance clock by window size and 100 seconds into second window
		mockClock.Add(time.Duration(winsz) * time.Minute)
		mockClock.Add(40 * time.Second)
		nextMaxAllowed = int(float32(100+(60*durationInStartWin)) / float32((60 * winsz)) * float32(threshold))
		sendParallel(w, mockClock)
		validate(nextMaxAllowed)
	}

}
