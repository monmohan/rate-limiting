# rate-limiting
A simple sliding window rate limiting implementation based on [blog by Cloudflare](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/#fn3)

Here is an simple excerpt of the idea -
* Assume that we want to limit our requests to 100 Requests per Minute
* Say a request arrives at Minute 23, Second 40
* If within the window Minute 22, Second 40 --- Window Minute 23, Second 40, total requests made is < 100, then this request should be allowed
* How do we know the total requests made in that window?
* This is the clever part-
* We will be counting requests per minute window , so we know that the number of requests made from Minute 23:00 -- Minute 23:40. Say it was 50
* Since we are counting requests per minute window then we have Minute 22:00 -- Minute 23:00 total count as well. Say it was 90 
* If we assumed that the requests were made at fixed rate then, from Minute 22:40 -- Minute 23:00, a duration of 20 seconds, the window would consume `90 * (20/60)` i.e. 30.
* Hence the total count in the window 22:40 - 23:40 = 30+50. So we are below the limit of 100 and the request should be allowed


