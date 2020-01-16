package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.
or 429 and Retry-After header
-- save/retrieve token in firestore
*/

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
