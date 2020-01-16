package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.
or 429 and Retry-After header
-- save/retrieve token in firestore
*/
var (
	firestoreClient *firestore.Client
	ctx             = context.Background()
	sessionSecret   = os.Getenv("SESSION_SECRET")
	customDomain    = os.Getenv("CUSTOM_DOMAIN")
	gcrDomain       = os.Getenv("GCR_DOMAIN")
)

func main() {
	defer firestoreClient.Close()
}

func init() {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Panic("GOOGLE_CLOUD_PROJECT must be set")
	}
	if checkNet() {
		log.Fatal("THERE IS NOTHING we can do without access to internet")
	}

	firestoreClient = initFirestoreDatabase(ctx)
	store := sessions.NewCookieStore([]byte(sessionSecret))

	router := gin.Default()
	router.Use(sessions.Sessions("go-spotify", store))

	router.Static("/static", "./static")
	router.StaticFile("/favicon.ico", "./favicon.ico")
	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "main.html", gin.H{})
	})
	router.GET("/callback", callback)
	router.GET("/login", login)

	router.Use(Redirector())

	authorized := router.Group("/")
	authorized.Use(AuthenticationRequired("/user"))
	{
		// moved to Cloud Function
		// authorized.POST("/recent", recent)
		// HTML pages
		authorized.GET("/top", top)
		authorized.GET("/popular", popular)
		authorized.GET("/chart", analysis)
		authorized.GET("/history", history)
		authorized.GET("/mood", moodFromHistory)
		authorized.GET("/user", user)
		// HIDDEN from menu
		authorized.GET("/logout", logout)
		// TODO - make useful
		authorized.GET("/tracks", tracks)
		authorized.GET("/playlists", playlists)
		authorized.GET("/albums", albums)
		// TXT pages TODO
		authorized.GET("/artists", artists)
		authorized.GET("/search", search)
		authorized.GET("/recommend", recommend)
		authorized.GET("/spot", spot)
		// DISABLED TODO - move to Cloud Function
		// authorized.GET("/midnight", midnight)
	}

	router.Run()
}

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
