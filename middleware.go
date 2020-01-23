package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
