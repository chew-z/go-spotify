package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const sessionTimeout = 3600

var (
	firestoreClient *firestore.Client
	ctx             = context.Background()
	sessionSecret   = os.Getenv("SESSION_SECRET")
	customDomain    = os.Getenv("CUSTOM_DOMAIN")
	gcrDomain       = os.Getenv("GCR_DOMAIN")
	redirectURI     = os.Getenv("REDIRECT_URI") // TODO generate callback URI
	projectID       = os.Getenv("GOOGLE_CLOUD_PROJECT")
	gae             = os.Getenv("GAE_ENV")
	gcr             = os.Getenv("GOOGLE_CLOUD_RUN")
	timezonesURL    = os.Getenv("TIMEZONES_CLOUD_FUNCTION")
)

func main() {
	defer firestoreClient.Close()
}

func init() {
	// Do some lazy initialization to speed up cold start
	go func() {
		if gcr == "YES" {
			log.Printf("Project ID: %s, service account email: %s", getProjectID(), getAccountEmail())
		}
		if checkNet() {
			log.Println("THERE IS NOTHING we can do without access to internet")
		}
	}()

	firestoreClient = initFirestoreDatabase(ctx)
	store := sessions.NewCookieStore([]byte(sessionSecret))

	router := gin.Default()
	router.Use(sessions.Sessions("go-spotify", store))
	// A zero/default http.Server, like the one used by the package-level helpers
	// http.ListenAndServe and http.ListenAndServeTLS, comes with no timeouts.
	// You don't want that.
	server := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  90 * time.Second,
	}
	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")
	router.Use(Headers()) // Custom headers middleware
	router.Static("/static", "./static")
	router.StaticFile("/favicon.ico", "./favicon.ico")
	router.StaticFile("/apple-touch-icon.png", "./static/apple-touch-icon.png")
	router.StaticFile("/apple-touch-icon-precomposed.png", "./static/apple-touch-icon-precomposed.png")
	// In real world we need rate limiting
	router.Use(RateLimiter(func(c *gin.Context) string {
		return c.ClientIP() // limit rate by client ip
	}, func(c *gin.Context) (*rate.Limiter, time.Duration) {
		return rate.NewLimiter(20.0, 40), time.Hour // limit 20 queries/ second / clientIp
		// and permit bursts of at most 40, and the limiter liveness time duration is 1 hour
	}, func(c *gin.Context) {
		c.AbortWithStatus(429) // handle exceed rate limit request
	}))

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "main.html", gin.H{
			"title": "music.suka.yoga",
		})
	})
	router.GET("/callback", callback)
	router.GET("/login", login)

	// Custom domain middleware
	router.Use(Redirector()) // middleware works for endpoints below
	// Authorization middleware
	authorized := router.Group("/")
	authorized.Use(AuthenticationRequired("/user"))
	{
		// HTML pages
		authorized.GET("/top", top)
		authorized.GET("/popular", popular)
		authorized.GET("/chart", chart)
		authorized.GET("/history", history)
		authorized.GET("/mood", moodFromHistory)
		authorized.GET("/playlists", playlists)
		authorized.GET("/albums", albums)
		authorized.GET("/user", user)
		// HIDDEN from menu
		authorized.GET("/logout", logout)
		authorized.GET("/playlisttracks", playlistTracks)
		authorized.GET("/albumtracks", albumTracks)
		// TODO - make useful
		// TXT pages TODO
		authorized.GET("/artists", artists)
		authorized.GET("/search", search)
		authorized.GET("/recommend", recommend)
	}
	// router.Run()
	server.ListenAndServe()
}
