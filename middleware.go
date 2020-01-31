package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
)

/*Redirector - middleware for redirecting CloudRun
to custom domain
*/
func Redirector() gin.HandlerFunc {
	return func(c *gin.Context) {
		if gcr == "YES" {
			if domain := c.Request.Host; domain == gcrDomain {
				url := fmt.Sprintf("https://%s%s", customDomain, c.Request.URL.Path)
				if qs := c.Request.URL.RawQuery; qs != "" {
					url += "?" + qs
				}
				defer func() {
					log.Printf("Redirector: redirecting to endpoint %s", url)
					c.Redirect(http.StatusSeeOther, url)
					c.Abort()
				}()
			}
		}
	}
}

/*Headers - middleware for adding custom
headers - also by path
*/
func Headers() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Service-Worker-Allowed", "/")
		if strings.HasPrefix(c.Request.RequestURI, "/static/") {
			c.Header("Cache-Control", "max-age=86400")
		}
	}
}

var limiterSet = cache.New(15*time.Minute, 3*time.Minute)

/*RateLimiter -a in-memory middleware to limit access rate
by custom key and rate
*/
func RateLimiter(key func(*gin.Context) string, createLimiter func(*gin.Context) (*rate.Limiter, time.Duration),
	abort func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := key(c)
		limiter, ok := limiterSet.Get(k)
		if !ok {
			var expire time.Duration
			limiter, expire = createLimiter(c)
			limiterSet.Set(k, limiter, expire)
		}
		ok = limiter.(*rate.Limiter).Allow()
		if !ok {
			c.Abort()
			return
		}
		c.Next()
	}
}
