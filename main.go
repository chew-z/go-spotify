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
	router.Use(favicon.New("./favicon.png"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is go-spotify here.")
	})
	router.GET("/callback", callback)

	router.GET("/user", user)
	router.GET("/top", top)
	router.GET("/recent", recent)
	router.GET("/history", history)
	// router.GET("/midnight", midnight)
	router.GET("/tracks", tracks)
	router.GET("/playlists", playlists)
	router.GET("/albums", albums)
	router.GET("/artists", artists)

	router.GET("/search", search)
	// router.GET("/mood", mood)
	router.GET("/mood", moodFromHistory)
	router.GET("/recommend", recommend)
	router.GET("/spot", spot)
	router.GET("/analyze", analyze)

	router.Run()
}
