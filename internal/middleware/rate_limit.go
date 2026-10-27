package middleware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"github.com/redis/go-redis/v9"
)

type RateLimitType int

const (
	RateLimitTypeAPI RateLimitType = iota
	RateLimitTypeUser
	RateLimitTypeIP
)

type RateLimitConfig struct {
	Key       string
	Rate      int
	Interval  time.Duration
	LimitType RateLimitType
	Message   string
	SSE       bool
}

const slidingWindowScript = `
local now = redis.call("TIME")
local nowMs = now[1] * 1000 + math.floor(now[2] / 1000)
local cutoff = nowMs - ARGV[2]
redis.call("ZREMRANGEBYSCORE", KEYS[1], 0, cutoff)
local count = redis.call("ZCARD", KEYS[1])
if count >= tonumber(ARGV[1]) then
  local first = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
  local retry = tonumber(ARGV[2])
  if #first >= 2 then
    retry = math.max(1, tonumber(ARGV[2]) - (nowMs - tonumber(first[2])))
  end
  return {0, count, retry}
end
local sequence = redis.call("INCR", KEYS[2])
local member = tostring(nowMs) .. ":" .. tostring(sequence)
redis.call("ZADD", KEYS[1], nowMs, member)
redis.call("PEXPIRE", KEYS[1], ARGV[2] + 1000)
redis.call("PEXPIRE", KEYS[2], ARGV[2] + 1000)
return {1, count + 1, 0}
`

func RateLimitMiddleware(redisClient *redis.Client, config RateLimitConfig) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if redisClient == nil || config.Rate <= 0 || config.Interval <= 0 {
			writeRateLimitError(c, config, "rate limit is not configured", 0)
			return
		}

		key, ok := rateLimitKey(c, config)
		if !ok {
			writeRateLimitError(c, config, "user identity is unavailable", 0)
			return
		}

		windowMs := config.Interval.Milliseconds()
		result, err := redisClient.Eval(
			ctx,
			slidingWindowScript,
			[]string{"rate_limit:" + key, "rate_limit:" + key + ":sequence"},
			config.Rate,
			windowMs,
		).Int64Slice()
		if err != nil {
			writeRateLimitError(c, config, "rate limit service unavailable", 0)
			return
		}
		if len(result) < 3 || result[0] == 0 {
			retryAfter := int(result[2] / 1000)
			if retryAfter <= 0 {
				retryAfter = 1
			}
			writeRateLimitError(c, config, config.Message, retryAfter)
			return
		}
		c.Next(ctx)
	}
}

func rateLimitKey(c *app.RequestContext, config RateLimitConfig) (string, bool) {
	prefix := strings.Trim(config.Key, ":")
	if prefix == "" {
		prefix = string(c.Request.URI().Path())
	}
	switch config.LimitType {
	case RateLimitTypeUser:
		value, exists := c.Get(constants.UserVoKey)
		user, valid := value.(*api.UserVo)
		if !exists || !valid || user == nil || user.ID <= 0 {
			return "", false
		}
		return fmt.Sprintf("%s:user:%d", prefix, user.ID), true
	case RateLimitTypeIP:
		return fmt.Sprintf("%s:ip:%s", prefix, c.ClientIP()), true
	default:
		return fmt.Sprintf("%s:api:%s:%s", prefix, c.Request.Header.Method(), c.Request.URI().Path()), true
	}
}

func writeRateLimitError(c *app.RequestContext, config RateLimitConfig, message string, retryAfter int) {
	if message == "" {
		message = "too many requests, please try again later"
	}
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}
	if config.SSE {
		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		writer := sse.NewWriter(c)
		lastEventID := sse.GetLastEventID(&c.Request)
		_ = writer.WriteEvent(lastEventID, "error", []byte(message))
		_ = writer.WriteEvent(lastEventID, "done", []byte("1"))
		c.Flush()
		c.Abort()
		return
	}
	c.JSON(consts.StatusTooManyRequests, map[string]any{
		"code":    40300,
		"message": message,
		"data":    nil,
	})
	c.Abort()
}
