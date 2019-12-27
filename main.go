package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/thinkerou/favicon"
	"github.com/zmb3/spotify"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.
*/

const (
	maxLists       = 5
	maxTracks      = 5
	cookieLifetime = 2
)

var (
	kaszka        = cache.New(60*time.Minute, 1*time.Minute)
	auth          = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed)
	clientChannel = make(chan *spotify.Client)
	redirectURI   = os.Getenv("REDIRECT_URI")
)

func main() {
}

func init() {
	router := gin.Default()
	router.Use(favicon.New("./favicon.png"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is go-spotify here.")
	})
	router.GET("/user", user)
	router.GET("/top", top)
	router.GET("/tracks", tracks)
	router.GET("/playlists", playlists)
	router.GET("/albums", albums)
	router.GET("/artists", artists)
	router.GET("/recent", recent)

	router.GET("/search", search)
	router.GET("/analyze", analyze)

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
	c.SetCookie("deuce", "1", cookieLifetime, endpoint, "", false, true)
	url := fmt.Sprintf("http://%s%s", c.Request.Host, endpoint)
	defer c.Redirect(303, url)
	log.Printf("/callback: redirecting to endpoint %s", url)
}

/*
 */
func user(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		msg := fmt.Sprintf("You are logged in as: %s", user.DisplayName)
		c.String(http.StatusOK, msg)
	}()
}

/* top - prints user top tracks (sensible defaults)
read zmb3/spotify code to learn more
*/
func top(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
func recent(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		recent, err := client.PlayerRecentlyPlayed()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Recently Played :")
		for _, item := range recent {
			b.WriteString("\n- ")
			b.WriteString(" [ ")
			b.WriteString(item.PlayedAt.Format("15:04:05"))
			b.WriteString(" ] ")
			b.WriteString(item.Track.Name)
			b.WriteString(" --  ")
			b.WriteString(item.Track.Artists[0].Name)
		}
		c.String(http.StatusOK, b.String())
	}()
}

func tracks(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		tracks, err := client.CurrentUsersTracks()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Tracks :")
		for _, item := range tracks.Tracks {
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
func playlists(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		playlists, err := client.CurrentUsersPlaylists()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Playlists:")
		for _, item := range playlists.Playlists {
			b.WriteString("\n- ")
			b.WriteString(item.Name)
		}
		c.String(http.StatusOK, b.String())
	}()
}
func albums(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		albums, err := client.CurrentUsersAlbums()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Albums:")
		for _, item := range albums.Albums {
			b.WriteString("\n- ")
			b.WriteString(item.Name)
			b.WriteString(" --  ")
			b.WriteString(item.Artists[0].Name)
		}
		c.String(http.StatusOK, b.String())
	}()
}
func artists(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		artists, err := client.CurrentUsersFollowedArtists()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var b strings.Builder
		b.WriteString("Artists:")
		for _, item := range artists.Artists {
			b.WriteString("\n- ")
			b.WriteString(item.Name)
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* search - searches playlists, albums, tracks etc.
 */
func search(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)
	qCookie, cookieErr := c.Cookie("search_query")
	cCookie, _ := c.Cookie("search_category")
	if cookieErr != nil {
		qCookie = "NotSet"
		cCookie = "NotSet"
		c.SetCookie("search_query", c.Query("q"), cookieLifetime, endpoint, "", false, true)
		c.SetCookie("search_category", c.Query("c"), cookieLifetime, endpoint, "", false, true)
	}
	log.Printf("Cookie values: %s %s \n", qCookie, cCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		query := c.DefaultQuery("q", qCookie)
		searchCategory := c.DefaultQuery("c", cCookie)
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

func analyze(c *gin.Context) {
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	deuceCookie, _ := c.Cookie("deuce")
	log.Printf("Deuce: %s ", deuceCookie)
	qCookie, cookieErr := c.Cookie("search_query")
	cCookie, _ := c.Cookie("search_category")
	if cookieErr != nil {
		qCookie = "NotSet"
		cCookie = "NotSet"
		c.SetCookie("search_query", c.Query("q"), cookieLifetime, endpoint, "", false, true)
		c.SetCookie("search_category", c.Query("c"), cookieLifetime, endpoint, "", false, true)
	}
	log.Printf("Cookie values: %s %s \n", qCookie, cCookie)

	if client == nil { // get client from oauth
		if deuceCookie == "1" { // wait for auth to complete
			client = <-clientChannel
			log.Printf("%s: Login Completed!", endpoint)
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
		query := c.DefaultQuery("q", qCookie)
		searchCategory := c.DefaultQuery("c", cCookie)
		searchType := searchType(searchCategory)
		results, err := client.Search(query, searchType)
		if err != nil {
			log.Println(err.Error())
			c.String(http.StatusNotFound, err.Error())
			return
		}
		resString := handleAudioFeatures(results, client)
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
			b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, strconv.Itoa(item.Popularity)))
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

func handleAudioFeatures(results *spotify.SearchResult, client *spotify.Client) string {
	var b strings.Builder

	b.WriteString("Analysis:\n")
	b.WriteString("Energy, Valence, Loud, Tempo, Acoustic, Instrumental, Dance, Speach\n")
	if results.Tracks != nil {
		tracks := results.Tracks.Tracks
		var tr []spotify.ID
		for _, item := range tracks {
			b.WriteString(fmt.Sprintf(" - %s - %s:\n", item.Name, item.Artists[0].Name))
			tr = append(tr, item.ID)
		}
		// using multiple track.IDs at once saves us many, many calls to Spotify
		audioFeatures, _ := client.GetAudioFeatures(tr...) // GetAudioFeatures has variadic argument
		for _, res := range audioFeatures {
			b.WriteString(fmt.Sprintf("  %.4f | %.4f | %.4f | %.4f |", res.Energy, res.Valence, res.Loudness, res.Tempo))
			b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Acousticness, res.Instrumentalness))
			b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Danceability, res.Speechiness))
			b.WriteString("\n")
			// b.WriteString(fmt.Sprintf("\n%v\n%v\n", res.AnalysisURL, res.TrackURL))
		}
	}
	if results.Playlists != nil {
		playlists := results.Playlists.Playlists
		for i, pl := range playlists {
			if i >= maxLists {
				break
			}
			playlist, _ := client.GetPlaylist(pl.ID)
			b.WriteString(fmt.Sprintf("\n  %s - %s\n", playlist.Name, playlist.Description))
			var tr []spotify.ID
			for j, item := range playlist.Tracks.Tracks {
				if j >= maxTracks {
					break
				}
				b.WriteString(fmt.Sprintf(" - %s - %s:\n", item.Track.Name, item.Track.Artists[0].Name))
				tr = append(tr, item.Track.ID)
			}
			// using multiple track.IDs at once saves us many, many calls to Spotify
			audioFeatures, _ := client.GetAudioFeatures(tr...) // GetAudioFeatures has variadic argument
			for _, res := range audioFeatures {
				b.WriteString(fmt.Sprintf("  %.4f | %.4f | %.4f | %.4f |", res.Energy, res.Valence, res.Loudness, res.Tempo))
				b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Acousticness, res.Instrumentalness))
				b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Danceability, res.Speechiness))
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}
