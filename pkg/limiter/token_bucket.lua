
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])


local state = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(state[1])
local last_refill = tonumber(state[2])

if tokens == nil then
    tokens = capacity
    last_refill = now
end

local elapsed = now - last_refill
if elapsed < 0 then
    elapsed = 0
end

local tokens_to_add = elapsed * rate
tokens = tokens + tokens_to_add

if tokens > capacity then
    tokens = capacity
end

local allowed = 0
local remaining = 0
local retry_after = 0
local reset_time = 0

if tokens >= cost then
    tokens = tokens - cost
    allowed = 1
    remaining = tokens
    reset_time = now
    
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
    
    local ttl = math.ceil((capacity / rate) * 2)
    redis.call('EXPIRE', key, ttl)
else
    allowed = 0
    remaining = tokens
    
    local needed = cost - tokens
    retry_after = needed / rate
    
    local to_full = capacity - tokens
    reset_time = now + (to_full / rate)
end

return {allowed, remaining, tostring(retry_after), tostring(reset_time)}