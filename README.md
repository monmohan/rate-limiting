# Rate-limiting
A simple __sliding window , fixed rate__ rate limiting implementation supporting flexible second and minute window sizes

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
The rate limiter supports window size of 1-30 seconds and 1-30 minutes. This allows both granular (persecond for example) rate limiting as well as broad window like 30 minutes. Usage is explained below -

### Single Minute Window
```go
  
  //Create a ratelimiter which will allow 10000 requests per minute
  rateLimiter := PerMinute(10000)
  
 /* Choose a store for counters, There are two OOTB, Memcached and local.
  * local is an In-Memory map, to be used only for testing. For production use Memcached. Its proven to work at scale.
  * Below is an example for using local memcached as backing counter store. 
  * In production, you would likely use a cluster or something like AWS Elasticache
  */
  rateLimiter.Store = &memcached.CounterStore{Client: memcache.New("127.0.0.1:11211")}
  
  //Now ratelimiter is ready to use, should "
  client:=Client("someKey") //UserID, ApplicationID, IP etc.
  allowed :=rateLimiter.Allow(client)
  
 /* Or Use AllowWithStats() which returns all stats based on which allow/deny flag was set by the rate limiter
  * Callers can examine stats like percentage window used etc.
  */
  stats :=ratelimiter.AllowWithStats(client)
  
  //If you print stats struct, you would get a JSON representatino like below 
  {"FormattedWindowTime":"H 10 M 54 S 10 MS 754","CurrentWindowIndex":54,"CurrentCounter":51,"PreviousWindowIndex":53,"PreviousWindowUsedPercent":0.8333333,"PreviousWindowUseCount":0,"RollingCounter":51,"Allow":false}
  
```

### Multi-Minute Window
```go
  //N=15, A rate limiter with 15 minute window thershold instead of 1
  rateLimiter := PerNMinute(threshold,15)
  //rest of the usage remains same as per-minute
  
  
```
### Per Second/Per N Seconds Window
```go
  //N=15, A rate limiter with 15 second window thershold
  rateLimiter := PerNSecond(threshold,15)
  //rest of the usage remains same ..
  
  
```
