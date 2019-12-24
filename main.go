package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/thinkerou/favicon"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

const (
	maxLists  = 5
	maxTracks = 5
)

var (
	// Create a cache with a default expiration time of 15 minutes
	kaszka      = cache.New(15*time.Minute, 1*time.Minute)
	redirectURI = os.Getenv("REDIRECT_URI")
	// client      spotify.Client
	router   *gin.Engine
	state    = "abc123"
	endpoint = "/whoami"
)

func main() {

}

func init() {
	// first start an HTTP server
	router = gin.Default()

	router.Use(favicon.New("./favicon.png"))
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is go-spotify here.")
	})
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	router.GET("/whoami", whoAmI)
	router.GET("/login", authorize)
	router.GET("/callback", callback)
	router.GET("/top", top)
	router.GET("usertoptracks", userTopTracks)
	router.GET("/search", search)
	router.GET("/analyze", analyze)
	router.GET("/reset", reset)

	router.Run() // listen and serve on 0.0.0.0:8080
	// For Google AppEngine
	// Handle all requests using net/http
	http.Handle("/", router)
}

func whoAmI(c *gin.Context) {
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		// url := fmt.Sprintf("http://%s%s", c.Request.Host, "/login")
		// log.Printf("callback: redirecting to endpoint %s", url)
		// // both approaches work but 2nd isn't rewriting url
		// // defer redirect until finish otherwise we are looping ...
		// defer c.Redirect(http.StatusMovedPermanently, url)
		authorize(c)
		return
	}

	user, err := client.CurrentUser() //if client is not declared panic and chaos ensues
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.String(http.StatusOK, fmt.Sprintln("Man, you are", user.ID))
}

func authorize(c *gin.Context) {
	kaszka.Delete("token")
	kaszka.Delete("client")
	auth := spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead)
	auth.SetAuthInfo(os.Getenv("SPOTIFY_ID"), os.Getenv("SPOTIFY_SECRET"))
	authURL := auth.AuthURL(state)
	log.Println("Auth URL:", authURL)
	defer c.Redirect(http.StatusMovedPermanently, authURL)
}

/* callback -
 */
func callback(c *gin.Context) {
	auth := spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate)
	auth.SetAuthInfo(os.Getenv("SPOTIFY_ID"), os.Getenv("SPOTIFY_SECRET"))
	tok, err := auth.Token(state, c.Request)
	if err != nil {
		log.Fatal(err.Error())
		c.String(http.StatusForbidden, "Couldn't get token")
	}
	if st := c.Query("state"); st != state {
		log.Fatalf("State mismatch: %s != %s\n", st, state)
		c.String(http.StatusNotFound, "State mismatch")
	}
	jsonToken, jsonErr := json.MarshalIndent(tok, "    ", "    ")
	if jsonErr != nil {
		log.Println(jsonErr.Error())
	}
	_ = ioutil.WriteFile("token.json", jsonToken, 0600)
	// log.Printf("callback: Token saved to file\n%s", string(jsonToken))
	kaszka.Set("token", tok, tok.Expiry.Sub(time.Now()))
	log.Println("callback: Token cached")
	// client = auth.NewClient(tok)
	url := fmt.Sprintf("http://%s%s", c.Request.Host, endpoint)
	log.Printf("callback: redirecting to endpoint %s", url)
	// both approaches work but 2nd isn't rewriting url
	// defer redirect until finish otherwise we are looping ...
	defer c.Redirect(http.StatusMovedPermanently, url)
	// c.Request.URL.Path = endpoint
	// router.HandleContext(c)
}

/* getClient - gets token from cache or file or if not present
   authorize app and return to the calling endpoint
   r and c are just to redirect to /auth
*/
func getClient() (*spotify.Client, error) {
	var tok *oauth2.Token
	var err error
	if gclient, foundClient := kaszka.Get("client"); foundClient {
		log.Println("Found cached client")
		client := gclient.(*spotify.Client)
		return client, nil
	}
	// if client is not cached
	log.Println("No cached client found. Looking for token in order to create Spotify client.")
	if gtoken, foundToken := kaszka.Get("token"); foundToken {
		log.Println("Found cached token")
		tok = gtoken.(*oauth2.Token)
	} else {
		log.Println("No cached token found. Looking for token saved in file token.json")
		tok, err = tokenFromFile("./token.json")
		if err != nil {
			log.Println("Not found token.json. Authenticating first")
			return nil, errors.New("Not found token.json. Authenticating first")
		}
	}
	// log.Printf("%v", *tok)
	log.Printf("Token will expire in %s", tok.Expiry.Sub(time.Now()).String())
	if tok.Expiry.Before(time.Now()) { // expired so let's update it
		log.Println("Token expired. Authenticating first")
		return nil, errors.New("Token expired. Authenticating first")
	}
	auth := spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate)
	auth.SetAuthInfo(os.Getenv("SPOTIFY_ID"), os.Getenv("SPOTIFY_SECRET"))
	client := auth.NewClient(tok)
	kaszka.Set("client", &client, tok.Expiry.Sub(time.Now()))
	log.Println("Client cached")
	return &client, nil
}

func reset(c *gin.Context) {
	kaszka.Delete("token")
	kaszka.Delete("client")
	_ = ioutil.WriteFile("token.json", []byte(""), 0600)

	c.String(http.StatusOK, "Reseted")
}

/* top -
 */
func top(c *gin.Context) {
	endpoint = "/top"
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		authorize(c)
		// url := fmt.Sprintf("http://%s%s", c.Request.Host, "/login")
		// log.Printf("callback: redirecting to endpoint %s", url)
		// // both approaches work but 2nd isn't rewriting url
		// // defer redirect until finish otherwise we are looping ...
		// defer c.Redirect(http.StatusMovedPermanently, url)
		return
	}
	top, err := client.CurrentUsersTopTracks()
	if err != nil {
		log.Println(err.Error())
		c.String(http.StatusNotFound, err.Error())
	} else {
		var b strings.Builder
		b.WriteString("Top :")
		for _, item := range top.Tracks {
			b.WriteString("\n- ")
			b.WriteString(item.Name)
			// b.WriteString(" : ")
			// b.WriteString(item.Owner.ID)
		}
		// endpoint = "/user"
		c.String(http.StatusOK, b.String())
	}
}
func userTopTracks(c *gin.Context) {
	endpoint = "/usertoptracks"
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		// url := fmt.Sprintf("http://%s%s", c.Request.Host, "/login")
		// log.Printf("callback: redirecting to endpoint %s", url)
		// // both approaches work but 2nd isn't rewriting url
		// // defer redirect until finish otherwise we are looping ...
		// defer c.Redirect(http.StatusMovedPermanently, url)
		authorize(c)
		return
	}

	data, err := client.CurrentUsersTopTracks()
	if err != nil {
		log.Println(err.Error())
		c.String(http.StatusForbidden, err.Error())
	} else {
		c.JSON(http.StatusOK, data)
	}
}

func search(c *gin.Context) {
	query := c.DefaultQuery("q", "ABBA")
	searchCategory := c.DefaultQuery("c", "track")
	endpoint = fmt.Sprintf("/search?q=%s&c=%s", query, searchCategory)
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		// url := fmt.Sprintf("http://%s%s", c.Request.Host, "/login")
		// log.Printf("callback: redirecting to endpoint %s", url)
		// // both approaches work but 2nd isn't rewriting url
		// // defer redirect until finish otherwise we are looping ...
		// defer c.Redirect(http.StatusMovedPermanently, url)
		authorize(c)
		return
	}

	searchType := searchType(searchCategory)
	results, err := client.Search(query, searchType)
	if err != nil {
		log.Println(err.Error())
		c.String(http.StatusNotFound, err.Error())
	}
	resString := handleSearchResults(results)
	// endpoint = "/user"
	c.String(http.StatusOK, resString)
}
func analyze(c *gin.Context) {
	query := c.DefaultQuery("q", "ABBA")
	searchCategory := c.DefaultQuery("c", "track")
	endpoint = fmt.Sprintf("/analyze?q=%s&c=%s", query, searchCategory)
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		// url := fmt.Sprintf("http://%s%s", c.Request.Host, "/login")
		// log.Printf("callback: redirecting to endpoint %s", url)
		// // both approaches work but 2nd isn't rewriting url
		// // defer redirect until finish otherwise we are looping ...
		// defer c.Redirect(http.StatusMovedPermanently, url)
		authorize(c)
		return
	}

	searchType := searchType(searchCategory)
	results, err := client.Search(query, searchType)
	if err != nil {
		log.Println(err.Error())
		c.String(http.StatusNotFound, err.Error())
	}
	resString := handleAudioFeatures(results)

	// endpoint = "/user"
	c.String(http.StatusOK, resString)
}

// Retrieves a token from a local JSON file.
// https://developers.google.com/tasks/quickstart/go
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func handleAudioFeatures(results *spotify.SearchResult) string {
	var b strings.Builder
	client, clientError := getClient()
	if clientError != nil {
		log.Println(clientError.Error())
		return ""
	}

	b.WriteString("Analysis:\n")
	b.WriteString("Energy, Valence, Loud, Tempo, Acoustic, Instrumental, Dance, Speach\n")
	if results.Tracks != nil {
		tracks := results.Tracks.Tracks
		var tr []spotify.ID
		for _, item := range tracks {
			b.WriteString(fmt.Sprintf(" - %s - %s:\n", item.Name, item.Artists[0].Name))
			tr = append(tr, item.ID)
		}
		audioFeatures, _ := client.GetAudioFeatures(tr...) // GetAudioFeatures has variadic argument
		for _, res := range audioFeatures {
			b.WriteString(fmt.Sprintf("  %.4f | %.4f | %.4f | %.4f |", res.Energy, res.Valence, res.Loudness, res.Tempo))
			// b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Acousticness, res.Instrumentalness))
			// b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Danceability, res.Speechiness))
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
			// playlist, _ := client.GetPlaylist(pl.Owner.ID, pl.ID)
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
				// b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Acousticness, res.Instrumentalness))
				// b.WriteString(fmt.Sprintf(" %.4f | %.4f |", res.Danceability, res.Speechiness))
				b.WriteString("\n")
			}
		}
	}
	return b.String()
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
