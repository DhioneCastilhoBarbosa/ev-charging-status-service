package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimitMiddleware returns a Gin middleware that limits requests per client IP.
// rateLimit is requests per second (e.g. 0.25 = 15 per minute), burst is max burst.
func RateLimitMiddleware(rateLimit float64, burst int) gin.HandlerFunc {
	type entry struct {
		limiter *rate.Limiter
		last    time.Time
	}
	var (
		mu      sync.Mutex
		clients = make(map[string]*entry)
	)
	// Limpa entradas antigas periodicamente
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for ip, e := range clients {
				if time.Since(e.last) > 5*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		e, ok := clients[ip]
		if !ok {
			e = &entry{limiter: rate.NewLimiter(rate.Limit(rateLimit), burst), last: time.Now()}
			clients[ip] = e
		}
		e.last = time.Now()
		lim := e.limiter
		mu.Unlock()

		if !lim.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}
