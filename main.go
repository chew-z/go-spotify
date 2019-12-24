package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/thinkerou/favicon"
	"github.com/zmb3/spotify"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope
*/

// const (
// )
var (
	kaszka        = cache.New(60*time.Minute, 1*time.Minute)
	auth          = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead)
	clientChannel = make(chan *spotify.Client)
	redirectURI   = os.Getenv("REDIRECT_URI")
)

func init() {
}

func main() {

	router := gin.Default()
	router.Use(favicon.New("./favicon.png"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is auth-gin-cache.go here.")
	})
	router.GET("/user", user)
	router.GET("/top", top)
	router.GET("/search", search)
	router.GET("/callback", callback)

	router.Run(":8080")
}

/* statefull authorization handler using channels
state = calling endpoint (which is intended use of scope)
caches client for as long as token is valid (1 hour for spotify)
no persistent storing of token, there is no need?
spotify stores persisten cookies behind our back so it is enough?
*/
func callback(c *gin.Context) {
	endpoint := c.Request.FormValue("state")
	log.Printf("/callback: endpoint: %s", endpoint)
	// Now we need different token for each endpoint = state. Sucks big way!
	tok, err := auth.Token(endpoint, c.Request)
	if err != nil {
		c.String(http.StatusForbidden, "Couldn't get token")
		log.Panic(err)
	}
	// create copy of gin.Context to be used inside the goroutine
	// cCopy := c.Copy()
	go func() {
		client := auth.NewClient(tok)
		log.Println("/callback: Login Completed!")
		kaszka.Set(endpoint, &client, tok.Expiry.Sub(time.Now()))
		log.Printf("/callback: Cached client for: %s", endpoint)
		clientChannel <- &client
	}()
	url := fmt.Sprintf("http://%s%s?deuce=1", c.Request.Host, endpoint)
	defer c.Redirect(303, url)
	log.Printf("/callback: redirecting to endpoint %s", url)
}

/*
 */
func user(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)

	if client == nil { // get client from oauth
		if d := c.DefaultQuery("deuce", "0"); d == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Println("/user: Login Completed!")
		} else { // redirect to auth URL and exit
			url := auth.AuthURL(endpoint)
			log.Printf("%s: redirecting to %s", endpoint, url)
			// HTTP standard does not pass through HTTP headers on an 302/301 directive
			// 303 is never cached and always is GET
			c.Redirect(303, url)
			return
		}
	}
	defer func() {
		// use the client to make calls that require authorization
		user, err := client.CurrentUser()
		if err != nil {
			log.Panic(err)
		}
		msg := fmt.Sprintf("You are logged in as: %s", user.ID)
		c.String(http.StatusOK, msg)
	}()
}

/* top - prints user top tracks (sensible defaults)
read zmb3/spotify code to learn more
*/
func top(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)

	if client == nil { // get client from oauth
		if d := c.DefaultQuery("deuce", "0"); d == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Println("/user: Login Completed!")
		} else { // redirect to auth URL and exit
			url := auth.AuthURL(endpoint)
			log.Printf("%s: redirecting to %s", endpoint, url)
			// HTTP standard does not pass through HTTP headers on an 302/301 directive
			// 303 is never cached and always is GET
			c.Redirect(303, url)
			return
		}
	}
	defer func() {
		// use the client to make calls that require authorization
		top, err := client.CurrentUsersTopTracks()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Top :")
		for _, item := range top.Tracks {
			b.WriteString("\n- ")
			b.WriteString(item.Name)
			b.WriteString(" [ ")
			b.WriteString(item.Album.Name)
			b.WriteString(" ] --  ")
			b.WriteString(item.Artists[0].Name)
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* search - searches playlists, albums, tracks etc.
 */
func search(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)

	if client == nil { // get client from oauth
		if d := c.DefaultQuery("deuce", "0"); d == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Println("/search: Login Completed!")
			// Edge case = WHAT TODO?
			// - redirects erase search params
			c.String(http.StatusOK, "Fix this edge case for /search")
		} else { // redirect to auth URL and exit
			url := auth.AuthURL(endpoint)
			log.Printf("%s: redirecting to %s", endpoint, url)
			// HTTP standard does not pass through HTTP headers on an 302/301 directive
			// 303 is never cached and always is GET
			c.Redirect(303, url)
			return
		}
	}
	defer func() {
		query := c.DefaultQuery("q", "ABBA")
		searchCategory := c.DefaultQuery("c", "track")
		searchType := searchType(searchCategory)
		results, err := client.Search(query, searchType)
		if err != nil {
			log.Println(err.Error())
			c.String(http.StatusNotFound, err.Error())
			return
		}
		resString := handleSearchResults(results)
		c.String(http.StatusOK, resString)
	}()
}

/* getClient - restore client for given state from cache
or return nil
*/
func getClient(endpoint string) *spotify.Client {
	if gclient, foundClient := kaszka.Get(endpoint); foundClient {
		log.Printf("Cached client found for: %s", endpoint)
		client := gclient.(*spotify.Client)
		if tok, err := client.Token(); err != nil {
			log.Panic(err)
		} else {
			log.Printf("Token will expire in %s", tok.Expiry.Sub(time.Now()).String())
		}
		return client
	}
	msg := fmt.Sprintf("No cached client found for: %s", endpoint)
	log.Println(msg)
	return nil
}
func handleSearchResults(results *spotify.SearchResult) string {
	var b strings.Builder
	// handle album results
	if results.Albums != nil {
		b.WriteString("\nAlbums:\n")
		for _, item := range results.Albums.Albums {
			b.WriteString(fmt.Sprintf("  %s - %s : %s\n", item.ID, item.Name, item.Artists[0].Name))
		}
	}
	// handle playlist results
	if results.Playlists != nil {
		b.WriteString("\nPlaylists:\n")
		for _, item := range results.Playlists.Playlists {
			b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Owner.DisplayName))
		}
	}
	// handle tracks results
	if results.Tracks != nil {
		b.WriteString("\nTracks:\n")
		for _, item := range results.Tracks.Tracks {
			b.WriteString(fmt.Sprintf("  %s - %s : %s\n", item.ID, item.Name, item.Album.Name))
		}
	}
	// handle artists results
	if results.Artists != nil {
		b.WriteString("\nArtists:\n")
		for _, item := range results.Artists.Artists {
			b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Popularity))
		}
	}
	return b.String()
}

func searchType(a string) spotify.SearchType {
	switch a {
	case "track":
		return spotify.SearchTypeTrack
	case "playlist":
		return spotify.SearchTypePlaylist
	case "album":
		return spotify.SearchTypeAlbum
	case "artist":
		return spotify.SearchTypeArtist
	default:
		return spotify.SearchTypeTrack
	}
}
