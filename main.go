package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
)

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
			// log.Panic("GOOGLE_CLOUD_PROJECT must be set")
		}
		if checkNet() {
			log.Fatal("THERE IS NOTHING we can do without access to internet")
		}
	}()

	firestoreClient = initFirestoreDatabase(ctx)
	store := sessions.NewCookieStore([]byte(sessionSecret))

	router := gin.Default()
	router.Use(sessions.Sessions("go-spotify", store))

	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")
	router.Static("/static", "./static")
	router.StaticFile("/favicon.ico", "./favicon.ico")
	router.StaticFile("/apple-touch-icon.png", "./static/apple-touch-icon.png")
	router.StaticFile("/apple-touch-icon-precomposed.png", "./static/apple-touch-icon-precomposed.png")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "main.html", gin.H{})
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
		authorized.GET("/chart", charts)
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
	}

	router.Run()
}
