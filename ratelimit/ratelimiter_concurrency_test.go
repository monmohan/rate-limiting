package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

func TestConcurrentCallers(t *testing.T) {
	fmt.Println("TestConcurrentCallers")
	threshold := 100
	w := getRateLimiter(threshold)
	numConcurCallers := 3
	done := make(chan int, numConcurCallers)
	err := 2 * numConcurCallers
	maxCalls := 2 * threshold //just to avoid deadlock

	sendRequest := func() {
		i := 0
		for i < maxCalls {
			result := w.AllowRequest()

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

func TestConcurrentSlidingMultiWindow(t *testing.T) {
	fmt.Println("TestConcurrentSlidingMultiWindow")
	threshold := 120
	w := getRateLimiter(threshold)
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
			result := w.AllowRequest()
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
	sendParallel(nextReqTime, sendRequest)
	validate(nextMaxAllowed)

	nextMaxAllowed = threshold - int(float32(5*totalInWindow2)/float32(6))
	sendParallel(nextReqTime, sendRequest)
	validate(nextMaxAllowed)

}
