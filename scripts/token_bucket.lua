
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])


local state = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(state[1])
local last_refill = tonumber(state[2])

-- 2. Initialize if bucket doesn't exist
if tokens == nil then
    tokens = capacity
    last_refill = now
end

-- 3. Calculate the "Lazy Refill"
-- This is the exact same logic as your Go code
local elapsed = now - last_refill
if elapsed < 0 then
    elapsed = 0
end

local tokens_to_add = elapsed * rate
tokens = tokens + tokens_to_add

if tokens > capacity then
    tokens = capacity
end

-- 4. The Decision
local allowed = 0
local remaining = 0
local retry_after = 0
local reset_time = 0

if tokens >= cost then
    -- ALLOW
    tokens = tokens - cost
    allowed = 1
    remaining = tokens
    reset_time = now -- Reset time logic can be complex, simplifying to 'now' for success or calculating full fill
    
    -- Update Redis
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
    
    -- Set Expiry: Clean up keys if user stops sending requests
    -- Expire after (Capacity / Rate) * 2 seconds to be safe
    local ttl = math.ceil((capacity / rate) * 2)
    redis.call('EXPIRE', key, ttl)
else
    -- DENY
    allowed = 0
    remaining = tokens
    
    -- Calculate Retry-After (Time to get enough tokens for cost)
    local needed = cost - tokens
    retry_after = needed / rate
    
    -- Calculate Reset-Time (Time until bucket is full)
    local to_full = capacity - tokens
    reset_time = now + (to_full / rate)
end

-- 5. Return the result as an array
-- [Allowed (1/0), Remaining (float), RetryAfter (float), ResetTime (float)]
return {allowed, remaining, retry_after, reset_time}