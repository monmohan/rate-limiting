# Rate-limiting
A simple __sliding window , fixed rate__ rate limiting implementation based on [blog by Cloudflare](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/#fn3)

## Overview
* Simple algorithm for rate limiting at scale
* Supports Global Rate Limiting 
    * The Rate Limiter applies a consistent rate limit to incoming requests regardless of which instance of Rate Limter processes the request. 
    * This is accomplished by sharing the state using a backing store (There is out of box support for memcached)
* Low memory and network overhead. 
    * Keeps two counters (32 bit integers) 
    * Two __contention-free__ network calls (Fetch and Increment)


### Explanation of how this works -
* Assume that we want to limit our requests to 100 Requests per Minute (rpm)
* Say a request arrives at Minute 23, Second 40 (23:40)
* If within the window Minute 22, Second 40 (22:40) --> Window Minute 23, Second 40 (23:40), total requests made is < 100, then this request should be allowed
* How do we know the total requests made in that window?
* This is the clever part-
* We count requests per minute window (0-1, 1-2, 2-3 and so on)
* That means we have the number of requests made from Minute 23:00 -- Minute 23:40. Say it was 50
* Since we are counting requests per minute window then we have Minute 22:00 -- Minute 23:00 total count as well. Say it was 90. 
* If we assumed that the requests were made at fixed rate then, from Minute 22:40 -- Minute 23:00, a duration of 20 seconds, the window would consume `90 * (20/60)` i.e. 30.
* Hence the total count in the window 22:40 - 23:40 ~= 30+50. So we are below the limit of 100 and the request should be allowed

## Using the Rate Limiter

### Single Minute Window
```go
  //Choose a store for counters, There are two OOTB, Memcached and local.
  //local is an In-Memory map, to be used only for testing. For production use Memcached. Its proven to work at scale
  //Below example for a local memcached. In production, you would use a cluster or something like AWS Elasticache
  rateLimiter := PerMinute("Somekey", threshold)
  rateLimiter.Store = &memcached.CounterStore{Client: memcache.New("127.0.0.1:11211")}
  
  //Now ratelimiter is ready to use
  allowed :=rateLimiter.Allow()
  
  //Or Rate Limiter with stats interface, callers can examine stats like percentage window used etc.
  stats :=ratelimiter.AllowWithStats()
  
  //here is how stats look
  fmt.Printf(stats)
  
  
```

### Multi-Minute Window
```go
  //N=15, A rate limiter with 15 minute window thershold instead of 1
  rateLimiter := PerNMinute("Somekey", threshold,15)
  //rest of the usage remains same as per-minute
  
  
```
