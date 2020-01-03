package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	// router.Use(favicon.New("./favicon.ico"))
	router.Static("/static", "./static")
	router.StaticFile("/favicon.ico", "./favicon.ico")
	// Process the templates at the start so that they don't have to be loaded
	// from the disk again. This makes serving HTML pages very fast.
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "main.html", gin.H{})
	})
	// internal pages
	router.GET("/callback", callback)
	router.GET("/recent", recent)
	// HTML pages
	router.GET("/top", top)
	router.GET("/popular", popular)
	router.GET("/history", history)
	router.GET("/mood", moodFromHistory)
	router.GET("/user", user)
	// TXT pages TODO
	router.GET("/tracks", tracks)
	router.GET("/playlists", playlists)
	router.GET("/albums", albums)
	router.GET("/artists", artists)
	router.GET("/search", search)
	router.GET("/recommend", recommend)
	router.GET("/spot", spot)
	router.GET("/analyze", analyze)
	// DISABLED
	// router.GET("/midnight", midnight)

	router.Run()
}
