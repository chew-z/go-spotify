package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.

-- save/retrieve token in firestore
*/
var (
	firestoreClient *firestore.Client
	ctx             = context.Background()
)

func main() {
	defer firestoreClient.Close()
}

func init() {
	firestoreClient = initFirestoreDatabase(ctx)

	router := gin.Default()
	store := sessions.NewCookieStore([]byte("sessionSuperSecret"))
	router.Use(sessions.Sessions("sessionName", store))

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
