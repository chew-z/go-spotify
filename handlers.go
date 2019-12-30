package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/zmb3/spotify"
	"google.golang.org/api/iterator"
)

/* TODO
-- gracefull handling of zmb3/spotify errors
like 403 lack of scope, unexpected endpoint etc.
*/

const (
	maxLists              = 5
	maxTracks             = 5
	cookieLifetime        = 10
	defaultMoodPlaylistID = "7vUhitas9hJkonwMx5t0z5"
)

var (
	countryPoland = "PL"
	kaszka        = cache.New(60*time.Minute, 1*time.Minute)
	auth          = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed, spotify.ScopePlaylistModifyPublic, spotify.ScopePlaylistModifyPrivate)
	clientChannel = make(chan *spotify.Client)
	redirectURI   = os.Getenv("REDIRECT_URI")
)

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

/* user - displays user identity (display name)
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

/* top - prints user's top tracks (sensible defaults)
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
			b.WriteString(joinArtists(item.Artists, ", "))
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* recent - display recently played tracks
 */
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
		recentlyPlayed, err := client.PlayerRecentlyPlayed()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		recent := normalizeRecentlyPlayed(recentlyPlayed)
		var b strings.Builder
		b.WriteString("Recently Played :")
		loc, _ := time.LoadLocation("Europe/Warsaw")
		for _, item := range recent {
			b.WriteString("\n- ")
			b.WriteString(" [ ")
			b.WriteString(item.PlayedAt.In(loc).Format("15:04:05"))
			b.WriteString(" ] ")
			b.WriteString(item.Track.Name)
			b.WriteString(" --  ")
			b.WriteString(joinArtists(item.Track.Artists, ", "))
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* store - save recently played tracks to Cloud Firestore database
 */
func store(c *gin.Context) {
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
		recentlyPlayed, err := client.PlayerRecentlyPlayed()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		recent := normalizeRecentlyPlayed(recentlyPlayed)
		// Use the application default credentials
		ctx := context.Background()
		conf := &firebase.Config{ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT")}
		app, err := firebase.NewApp(ctx, conf)
		if err != nil {
			log.Println(err.Error())
		}

		firestoreClient, err := app.Firestore(ctx)
		if err != nil {
			log.Println(err.Error())
		}
		defer firestoreClient.Close()

		var b strings.Builder
		b.WriteString("Recently Played :")
		loc, _ := time.LoadLocation("Europe/Warsaw")
		for _, item := range recent {
			artists := joinArtists(item.Track.Artists, ", ")
			playedAt := item.PlayedAt.In(loc).Format("15:04:05")

			b.WriteString("\n- ")
			b.WriteString(" [ ")
			b.WriteString(playedAt)
			b.WriteString(" ] ")
			b.WriteString(item.Track.Name)
			b.WriteString(" --  ")
			b.WriteString(artists)

			// _, _, err = firestoreClient.Collection("recently_played").Add(ctx, map[string]interface{}{
			_, err = firestoreClient.Collection("recently_played").Doc(string(item.Track.ID)).Set(ctx, map[string]interface{}{
				"played_at":  playedAt,
				"track_name": item.Track.Name,
				"artists":    artists,
			})
			if err != nil {
				log.Println(err.Error())
			}
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* restore - read saved tracks from Cloud Firestore database
 */
func restore(c *gin.Context) {
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
		// Use the application default credentials
		ctx := context.Background()
		conf := &firebase.Config{ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT")}
		app, err := firebase.NewApp(ctx, conf)
		if err != nil {
			log.Println(err.Error())
		}

		firestoreClient, err := app.Firestore(ctx)
		if err != nil {
			log.Println(err.Error())
		}
		defer firestoreClient.Close()
		type FirestoreTrack struct {
			Name     string `firestore:"track_name"`
			Artists  string `firestore:"artists"`
			PlayedAt string `firestore:"played_at"`
		}
		var b strings.Builder
		b.WriteString("Recently Played :\n")
		// iter := firestoreClient.Collection("recently_played").Documents(ctx)
		iter := firestoreClient.Collection("recently_played").OrderBy("played_at", firestore.Desc).Limit(100).Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Println(err.Error())
			}
			var tr FirestoreTrack
			if err := doc.DataTo(&tr); err != nil {
				log.Println(err.Error())
			}
			b.WriteString(fmt.Sprintf("[ %s ] %s -- %s\n", tr.PlayedAt, tr.Name, tr.Artists))
			// for key, value := range doc.Data() {
			// 	dt := fmt.Sprintf("%s=\"%s\"\n", key, value)
			// 	b.WriteString(dt)
			// }
		}

		c.String(http.StatusOK, b.String())

	}()
}

/* tracks - display some of user's tracks
 */
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
			b.WriteString(joinArtists(item.Artists, ", "))
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* playlists - display some of user's playlists
 */
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

/* albums - display some of user's albums
 */
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
			b.WriteString(joinArtists(item.Artists, ", "))
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* artists - displays user followed artists
 */
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

/* search - searches for playlists, albums, tracks etc.
 */
func search(c *gin.Context) {
	deuceCookie, _ := c.Cookie("deuce")
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
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

/* analyze - search for tracks and display
analysis of results
*/
func analyze(c *gin.Context) {
	deuceCookie, _ := c.Cookie("deuce")
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
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
		resString := handleAudioFeatures(results, client)
		c.String(http.StatusOK, resString)
	}()
}

/* recommend songs based on given tracks (maximum 5)
accepts query parameters t1..t5 with trackIDs.
prints recommended tracks
*/
func recommend(c *gin.Context) {
	deuceCookie, _ := c.Cookie("deuce")
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	log.Printf("Deuce: %s ", deuceCookie)
	for i := 1; i < 6; i++ {
		cookieName := fmt.Sprintf("t%d", i)
		c.SetCookie(cookieName, c.Query(cookieName), 45, endpoint, "", false, true)
		log.Printf("Cookie %s value: %s \n", cookieName, c.Query(cookieName))
	}

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
		trackIDs := []spotify.ID{}
		for i := 1; i < 6; i++ {
			cookieName := fmt.Sprintf("t%d", i)
			if track, err := c.Cookie(cookieName); err == nil {
				if len(track) > 0 {
					trackID := spotify.ID(track)
					log.Printf("Track cookie %s value: %v \n", cookieName, trackID)
					trackIDs = appendIfUnique(trackIDs, trackID)
				}
			}
		}
		log.Printf("%v", trackIDs)
		//Build recommend Request
		seeds := spotify.Seeds{
			Artists: []spotify.ID{},
			Tracks:  trackIDs,
			Genres:  []string{},
		}
		recs, err := client.GetRecommendations(seeds, nil, nil)
		if err != nil {
			log.Println(err.Error())
			c.String(http.StatusNotFound, err.Error())
			return
		}
		var b strings.Builder
		b.WriteString("Recommended tracks based on following tracks\n")
		seedTracks, errFull := fullTrackGetMany(client, trackIDs)
		if errFull != nil {
			log.Println(err.Error())
		} else {
			b.WriteString("Seeds:\n")
			for _, item := range seedTracks {
				b.WriteString(fmt.Sprintf(" * %s - %s : %s\n", item.ID, item.Name, joinArtists(item.Artists, ", ")))
			}
		}
		b.WriteString("---/---\n")
		for _, item := range recs.Tracks {
			b.WriteString(fmt.Sprintf("  %s - %s : %s\n", item.ID, item.Name, joinArtists(item.Artists, ", ")))
		}
		c.String(http.StatusOK, b.String())
	}()
}

/* spot - recommend tracks based on user top artists
recommeded tracks could replace default mood playlist
or any other (based on passed parameters)
r=1 - replace, p=[ID]
*/
func spot(c *gin.Context) {
	deuceCookie, _ := c.Cookie("deuce")
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	log.Printf("Deuce: %s ", deuceCookie)
	replaceCookie, cookieErr := c.Cookie("replace_playlist")
	playlistCookie, _ := c.Cookie("playlist_ID")
	if cookieErr != nil {
		c.SetCookie("replace_playlist", c.Query("r"), cookieLifetime, endpoint, "", false, true)
		c.SetCookie("playlist_ID", c.DefaultQuery("p", defaultMoodPlaylistID), cookieLifetime, endpoint, "", false, true)
	}
	log.Printf("Cookie values: %s %s \n", replaceCookie, playlistCookie)

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
		spotTracks, err := recommendFromTop(client)
		if err != nil {
			log.Println(err.Error())
			c.String(http.StatusNotFound, err.Error())
			return
		}
		var b strings.Builder
		b.WriteString("Recommended tracks based on your top artists\n")
		replace := c.DefaultQuery("r", replaceCookie)
		playlist := c.DefaultQuery("p", playlistCookie)
		if replace == "1" {
			recommendedPlaylistID := spotify.ID(playlist)
			chunks := chunkIDs(getSpotifyIDs(spotTracks), 100)
			err = client.ReplacePlaylistTracks(recommendedPlaylistID, chunks[0]...)

			if err == nil {
				log.Println("Tracks added")
			} else {
				log.Println(err)
			}
		} else {
			b.WriteString("Printing only, pass params (r=1, p=playlistID) if you wish to replace with recommended tracks\n")
		}
		for _, item := range spotTracks {
			b.WriteString(fmt.Sprintf("  %s - %s : %s (%d)\n", item.ID, item.Name, joinArtists(item.Artists, ", "), item.Popularity))
		}

		c.String(http.StatusOK, b.String())
	}()
}

/* mood - recommends tracks based on current mood
(recently played tracks)
recommeded tracks could replace default mood playlist
or any other (based on passed parameters)
r=1 - replace, p=[ID]
*/
func mood(c *gin.Context) {
	deuceCookie, _ := c.Cookie("deuce")
	endpoint := c.Request.URL.Path
	client := getClient(endpoint)
	log.Printf("Deuce: %s ", deuceCookie)
	replaceCookie, cookieErr := c.Cookie("replace_playlist")
	playlistCookie, _ := c.Cookie("playlist_ID")
	if cookieErr != nil {
		c.SetCookie("replace_playlist", c.Query("r"), cookieLifetime, endpoint, "", false, true)
		c.SetCookie("playlist_ID", c.DefaultQuery("p", defaultMoodPlaylistID), cookieLifetime, endpoint, "", false, true)
	}
	log.Printf("Cookie values: %s %s \n", replaceCookie, playlistCookie)

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
		spotTracks, err := recommendFromMood(client)
		if err != nil {
			log.Println(err.Error())
			c.String(http.StatusNotFound, err.Error())
			return
		}
		var b strings.Builder
		b.WriteString("Recommended tracks based on recently listened to\n")
		replace := c.DefaultQuery("r", replaceCookie)
		playlist := c.DefaultQuery("p", playlistCookie)
		if replace == "1" {
			recommendedPlaylistID := spotify.ID(playlist)
			chunks := chunkIDs(getSpotifyIDs(spotTracks), 100)
			err = client.ReplacePlaylistTracks(recommendedPlaylistID, chunks[0]...)
			if err == nil {
				log.Println("Tracks added")
			} else {
				log.Println(err.Error())
			}
		} else {
			b.WriteString("Printing only, pass params (r=1, p=playlistID) if you wish to replace with recommended tracks\n")
		}
		for _, item := range spotTracks {
			b.WriteString(fmt.Sprintf("  %s - %s : %s (%d)\n", item.ID, item.Name, joinArtists(item.Artists, ", "), item.Popularity))
		}

		c.String(http.StatusOK, b.String())
	}()
}
