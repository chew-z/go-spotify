package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thinkerou/favicon"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.

-- save/retrieve token in firestore
*/

func main() {
}

func init() {
	router := gin.Default()
	router.Use(favicon.New("./favicon.ico"))

	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is go-spotify here.")
	})
	router.GET("/callback", callback)

	router.GET("/user", user)
	router.GET("/top", top)
	router.GET("/recent", recent)
	router.GET("/history", history)
	router.GET("/popular", popular)
	// router.GET("/midnight", midnight)
	router.GET("/tracks", tracks)
	router.GET("/playlists", playlists)
	router.GET("/albums", albums)
	router.GET("/artists", artists)

	router.GET("/search", search)
	router.GET("/oldmood", mood)
	router.GET("/mood", moodFromHistory)
	router.GET("/recommend", recommend)
	router.GET("/spot", spot)
	router.GET("/analyze", analyze)

	router.Run()
}
