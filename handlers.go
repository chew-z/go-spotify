package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	spotify "github.com/chew-z/spotify"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	guuid "github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"google.golang.org/api/iterator"
)

const (
	maxLists       = 5
	maxTracks      = 5
	pageLimit      = 24
	cookieLifetime = 15
	//TODO - store for user, or change logic
	googleRootCertURL = "https://www.googleapis.com/oauth4/v3/certs"
)

var (
	kaszka = cache.New(20*time.Minute, 3*time.Minute)
	// Warning token will fail if you are changing scope (even if you narrow it down) so you might end up with bunch
	// of useless stored tokens that will keep failing
	// TODO - procedure for clearing useless token (users will have to re-authorize with Spotify)
	auth = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadEmail, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed, spotify.ScopePlaylistModifyPublic, spotify.ScopePlaylistModifyPrivate, spotify.ScopePlaylistReadCollaborative, spotify.ScopePlaylistReadPrivate) // clientChannel = make(chan *spotify.Client)
)

/* statefull authorization handler using channels
state = calling endpoint (which is intended use of scope)
caches client for as long as token is valid (3 hour for spotify)
no persistent storing of token, there is no need?
spotify stores persisten cookies behind our back so it is enough?
*/
func callback(c *gin.Context) {
	endpoint := c.Request.FormValue("state")
	log.Printf("/callback: endpoint: %s", endpoint)
	tok, err := auth.Token(endpoint, c.Request)
	if err != nil {
		c.String(http.StatusForbidden, "Couldn't get token")
		log.Panic(err)
	}
	uuid := guuid.New().String()
	go func() {
		spotifyClient := auth.NewClient(tok)
		log.Println("/callback: Login Completed!")
		// This is just a trick for passing client further (to /login endpoint) to get and save Spotify userID
		kaszka.Set(uuid, &spotifyClient, cache.DefaultExpiration)
		log.Printf("/callback: Cached client for: %s", endpoint)
		// clientChannel <- &spotifyClient
	}()
	url := fmt.Sprintf("http://%s%s?endpoint=%s&id=%s", customDomain, "/login", endpoint, uuid)
	defer func() {
		log.Printf("callback: redirecting to endpoint %s", url)
		c.Redirect(http.StatusFound, url)
	}()
}

/* login - This endpoint completes authorization process
it takes over from /callback, gets Spotify user, saves session variables
and saves token into database. Finaly it redirects to /user
TODO - separate final endpoint from authPath parameter
TODO - analyze what if it fails, is canceled? and we are left with token
but not session vars or session vars but no token saved?
*/
func login(c *gin.Context) {
	// This is where to we shall redirect after finishing login process
	// but also session authPath variable - endpoints for which user
	// is authorized
	// default is "/user"
	endpoint := c.Query("endpoint")
	uuid := c.Query("id") // create session unique id
	// /callback should have stored Spotify client in cache
	if gclient, foundClient := kaszka.Get(uuid); foundClient {
		log.Printf("/login: Cached client found for: %s", uuid)
		// get Spotify client
		spotifyClient := gclient.(*spotify.Client)
		// and get Spotify user (user.ID)
		user, err := spotifyClient.CurrentUser()
		if err != nil {
			log.Panic(err)
		}
		// get token for client
		newToken, _ := spotifyClient.Token()
		log.Println(newToken.Expiry.Sub(time.Now()))
		// path := fmt.Sprintf("users/%s/tokens%s", string(user.ID), endpoint)
		// save token to Firestore
		var newTok firestoreToken
		newTok.user = string(user.ID)
		newTok.displayname = user.DisplayName
		newTok.email = user.Email
		newTok.country = string(user.Country)
		newTok.path = endpoint
		newTok.token = newToken
		saveTokenToDB(&newTok)
		//Initialize history (don't wait) (must have token saved into firestore)
		go cloudRecent(string(user.ID))
		// save necessary variables into session
		session := sessions.Default(c)
		// TODO - is it necessary and what would be optimal?

		sessions.Default(c).Options(sessions.Options{MaxAge: sessionTimeout}) // make a session timeout after X seconds of inactivity
		log.Printf("/login: %s from %s", string(user.ID), string(user.Country))
		session.Set("user", string(user.ID))
		session.Set("email", user.Email)
		session.Set("displayname", user.DisplayName)
		session.Set("country", string(user.Country))
		session.Set("authPath", endpoint)
		session.Set("uuid", uuid)
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"login": "failed to set session values"})
			return
		}
		url := fmt.Sprintf("http://%s%s", c.Request.Host, endpoint)
		c.Redirect(http.StatusSeeOther, url)
		return
	}
	c.JSON(http.StatusTeapot, gin.H{"login": "failed to find cached client"})
}

/*logout - simplistic logout
TODO - hidden - make useful logout flow
user can always clear cookies or de-authorize app in Spotify setting
and we cannot clear Spotify cookies, beside it would log user out of web players etc.
It is however usefull as is - resets user session after deploying new version changing
session vars, otherwise we have Panic on casting string on nil interface
*/
func logout(c *gin.Context) {
	// without clearing Spotify cookie we will be simply re-logged transparently
	session := sessions.Default(c)
	session.Clear() // issue #91
	session.Save()
	log.Printf("/logout: %s", "bye")
	url := fmt.Sprintf("http://%s%s", c.Request.Host, "/")
	c.Redirect(305, url)
}

/* top - prints user's top tracks (sensible defaults)
read zmb5/spotify code to learn more
*/
func top(c *gin.Context) {
	spotifyClient := clientMagic(c)
	if spotifyClient != nil {
		// use the client to make calls that require authorization
		top, err := spotifyClient.CurrentUsersTopTracks()
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var tt topTrack
		var tracks []topTrack
		for _, item := range top.Tracks {
			tt.Name = item.Name
			tt.Album = item.Album.Name
			tt.Artists = joinArtists(item.Artists, ", ")
			tt.URL = item.ExternalURLs["spotify"]
			tt.Image = item.Album.Images[0].URL
			tracks = append(tracks, tt)
		}
		c.HTML(
			http.StatusOK,
			"top.html",
			gin.H{
				"Tracks": tracks,
				"title":  "Top tracks",
			},
		)
		return
	}
	c.JSON(http.StatusTeapot, gin.H{"/top": "failed to find  client"})
}

/* popular - read counter of how many tracks has been played from Firestore
and get us sorted list of most popular tracks.
I see three different ways of doing it:
3) getting tracks from firestore without calling Spotify API at all
4) with single call to Spotify API and two loops - GetTracks(ids ...ID)
5) with single loop and multiple calls to Spotify API - GetTrack(id ID)
*/
func popular(c *gin.Context) {
	spotifyClient := clientMagic(c)
	if spotifyClient != nil {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			u, err := spotifyClient.CurrentUser()
			if err != nil {
				log.Panic(err)
			}
			user = string(u.ID)
		}
		path := fmt.Sprintf("users/%s/popular_tracks", user)
		pops := firestoreClient.Collection(path).OrderBy("count", firestore.Desc).Limit(pageLimit).Documents(ctx)
		var pt popularTrack
		var toplist []int
		trackIDs := []spotify.ID{}
		for {
			doc, err := pops.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Println(err.Error())
			}
			trackID := spotify.ID(doc.Ref.ID)
			trackIDs = append(trackIDs, trackID)
			if err := doc.DataTo(&pt); err != nil {
				log.Println(err.Error())
			}
			toplist = append(toplist, pt.Count)
		}
		topTracks, err := fullTrackGetMany(spotifyClient, trackIDs)
		if err != nil {
			log.Println(err.Error())
		}
		var tt topTrack
		var tracks []topTrack
		for i := range toplist {
			tt.Count = toplist[i]
			tt.Name = topTracks[i].Name
			tt.Artists = joinArtists(topTracks[i].Artists, ", ")
			tt.URL = topTracks[i].ExternalURLs["spotify"]
			tt.Image = topTracks[i].Album.Images[0].URL
			tracks = append(tracks, tt)
		}
		// Call the HTML method of the Context to render a template
		c.HTML(
			http.StatusOK,
			"popular.html",
			gin.H{
				"Tracks": tracks,
				"title":  "Popular tracks",
			},
		)
		return
	}
	c.JSON(http.StatusTeapot, gin.H{"/popular": "failed to find  client"})
}

/* history - read saved tracks from Cloud Firestore database
 */
func history(c *gin.Context) {
	endpoint := c.Request.URL.Path
	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"/history": "failed to find  client"})
		return
	}
	page := c.Query("page")
	nav := getNavigation(page)

	defer func() {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			u, err := spotifyClient.CurrentUser()
			if err != nil {
				log.Panic(err)
			}
			user = string(u.ID)
		}
		path := fmt.Sprintf("users/%s/recently_played", user)
		q := paginateHistory(page, path, c)
		docs, err := q.Documents(ctx).GetAll()
		if err != nil {
			log.Println(err.Error())
		}
		lastDoc := docs[len(docs)-1].Data()["played_at"].(time.Time)
		c.SetCookie("lastDoc", lastDoc.String(), 1200, endpoint, "", false, true)
		c.SetCookie("lastPage", page, 1200, endpoint, "", false, true)
		var tr firestoreTrack
		var tracks []firestoreTrack
		if len(docs) < pageLimit {
			nav.Next = ""
		}
		for _, doc := range docs {
			if err := doc.DataTo(&tr); err != nil {
				log.Println(err.Error())
			} else {
				tracks = append(tracks, tr)
			}
		}
		nav.Endpoint = endpoint
		c.HTML(
			http.StatusOK,
			"history.html",
			gin.H{
				"Tracks":     tracks,
				"title":      "Recently Played",
				"Navigation": nav,
			},
		)
	}()
}

/* moodFromHistory - recommends tracks based on current mood
(recently played tracks) [Firestore version]
recommeded tracks could replace default mood playlist
or any other (based on passed parameters)
r=3 - replace, p=[ID]
TODO - defaultMoodPlaylistID - create new or store default in DB for user
*/
func moodFromHistory(c *gin.Context) {
	endpoint := c.Request.URL.Path
	spotifyClient := clientMagic(c)
	var recommendedTracks []spotify.FullTrack
	var err error
	if spotifyClient != nil {
		if replace := c.Query("r"); replace == "1" { // if Save button
			if y, found := kaszka.Get("tracks_" + endpoint); found { // get from cache
				log.Printf("tracks_%s found", endpoint)
				recommendedTracks = y.([]spotify.FullTrack)
			} else { // or generate new recommendations if cache empty
				recommendedTracks, err = recommendFromHistory(spotifyClient, c)
				if err != nil {
					log.Println(err.Error())
					c.String(http.StatusNotFound, err.Error())
				}
			}
			// get track IDs for created playlist
			chunks := chunkIDs(getSpotifyIDs(recommendedTracks), pageLimit)
			// and do the hops to create playlist and save tracks
			user, err := spotifyClient.CurrentUser()
			if err == nil {
				log.Printf("User found %s", user.DisplayName)
			} else {
				log.Println(err.Error())
			}
			location, _ := time.LoadLocation("Europe/Warsaw") // TODO
			playlist, err := spotifyClient.CreatePlaylistForUser(
				user.ID,
				fmt.Sprintf("Mood %s", time.Now().In(location).Format("Monday Jan _2 15:04")),
				"Generated by music.suka.yoga",
				false)
			if err == nil {
				log.Printf("Playlist created %s", playlist.ID.String())
			} else {
				log.Println(err.Error())
			}
			recommendedPlaylistID := spotify.ID(playlist.SimplePlaylist.ID)
			for _, chunk := range chunks {
				err = spotifyClient.ReplacePlaylistTracks(recommendedPlaylistID, chunk...)
				if err == nil {
					log.Println("Tracks added")
				} else {
					log.Println(err.Error())
				}
			}
		} else {
			// get recommendation (no saving)
			recommendedTracks, err = recommendFromHistory(spotifyClient, c)
			if err != nil {
				log.Println(err.Error())
				c.String(http.StatusNotFound, err.Error())
			}
		}
		kaszka.SetDefault("tracks_"+endpoint, recommendedTracks)
		// display tracks
		var tt topTrack
		var tracks []topTrack
		for _, item := range recommendedTracks {
			tt.Name = item.Name
			tt.Album = item.Album.Name
			tt.Artists = joinArtists(item.Artists, ", ")
			tt.URL = item.ExternalURLs["spotify"]
			tt.Image = item.Album.Images[0].URL
			tracks = append(tracks, tt)
		}
		c.HTML(
			http.StatusOK,
			"mood.html",
			gin.H{
				"Tracks": tracks,
				"title":  "Mood",
			},
		)
		return
	}
	c.JSON(http.StatusTeapot, gin.H{endpoint: "failed to find Spotify client"})
}

/* user - displays user identity (display name)
 */
func user(c *gin.Context) {
	spotifyClient := clientMagic(c)
	var User userLocation
	if spotifyClient != nil {
		user, err := spotifyClient.CurrentUser()
		if err != nil {
			log.Println(err.Error())
		}
		User.Country = user.Country
		User.Name = user.DisplayName
		User.URL = user.ExternalURLs["spotify"]
		dsnap, err := firestoreClient.Collection("users").Doc(user.ID).Get(ctx)
		if err != nil {
			log.Printf("Error retrieving token from Firestore for %s %s.\nPossibly it ain't there..", user.ID, err.Error())
		}
		tok := dsnap.Data()
		u := tok["premium_user"]
		if u == nil {
			User.Premium = false
		} else {
			User.Premium = u.(bool)
		}
		c.HTML(
			http.StatusOK,
			"user.html",
			gin.H{
				"User": User,
			},
		)
		return
	}
	c.String(http.StatusTeapot, "I am a teapot, that's all I know")
}

/*chart - present audio features for tracks from history/album/playlist
as radar or pie. History tracks are taken form firestore rather
then directly from spotify history.
*/
func chart(c *gin.Context) {
	endpoint := c.Request.URL.Path
	page := c.Query("page")
	chart := c.DefaultQuery("chart", "pie.html")
	pl := c.Query("pl")
	al := c.Query("al")
	nav := getNavigation(page)

	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{endpoint: "failed to get Spotify client"})
		return
	}
	{
		session := sessions.Default(c)
		trackIDs := []spotify.ID{}
		if pl != "" {
			options := new(spotify.Options)
			land := session.Get("country")
			if land != nil {
				country := land.(string)
				options.Country = &country
			}
			offset, _ := strconv.Atoi(page)
			offset = offset * pageLimit
			options.Offset = &offset
			limit := pageLimit
			options.Limit = &limit
			fields := "items(track(id))"
			itemsID := spotify.ID(pl)
			tracks, err := spotifyClient.GetPlaylistTracksOpt(itemsID, options, fields)
			if err != nil {
				log.Println(err.Error())
				c.String(http.StatusNotFound, err.Error())
			}
			for _, item := range tracks.Tracks {
				if item.Track.ID == "" {
					continue
				}
				trackID := item.Track.ID
				trackIDs = append(trackIDs, trackID)
			}
			if len(tracks.Tracks) < pageLimit {
				nav.Next = ""
			}
			nav.Back = fmt.Sprintf("/playlisttracks?pl=%s", pl)
		} else if al != "" {
			itemsID := spotify.ID(al)
			offset, _ := strconv.Atoi(page)
			offset = offset * pageLimit
			tracks, err := spotifyClient.GetAlbumTracksOpt(itemsID, pageLimit, offset)
			if err != nil {
				log.Println(err.Error())
				c.String(http.StatusNotFound, err.Error())
			}
			for _, item := range tracks.Tracks {
				if item.ID == "" {
					continue
				}
				trackID := item.ID
				trackIDs = append(trackIDs, trackID)
			}
			if len(tracks.Tracks) < pageLimit {
				nav.Next = ""
			}
			nav.Back = fmt.Sprintf("/albumtracks?al=%s", al)
		} else {
			user := session.Get("user")
			if user == nil {
				u, err := spotifyClient.CurrentUser()
				if err != nil {
					log.Println(err.Error())
					c.JSON(http.StatusTeapot, gin.H{endpoint: "failed to get Spotify user"})
					return
				}
				user = string(u.ID)
			}
			path := fmt.Sprintf("users/%s/recently_played", user)
			q := paginateHistory(page, path, c)
			docs, err := q.Documents(ctx).GetAll()
			if err != nil {
				log.Println(err.Error())
			}
			lastDoc := docs[len(docs)-1].Data()["played_at"].(time.Time)
			c.SetCookie("lastDoc", lastDoc.String(), 1200, endpoint, "", false, true)
			for _, doc := range docs {
				trackID := spotify.ID(doc.Ref.ID)
				trackIDs = append(trackIDs, trackID)
			}
			nav.Back = "/history"
		}
		data := miniAudioFeatures(trackIDs, spotifyClient) // uses pageLimit
		c.SetCookie("lastPage", page, 1200, endpoint, "", false, true)
		c.HTML(
			http.StatusOK,
			chart, //TODO - pie or radar
			gin.H{
				"title":      "Chart",
				"Navigation": nav,
				"Data":       data,
			},
		)
	}
}

/* tracks - display tracks for a playlist
 */
func playlistTracks(c *gin.Context) {
	endpoint := c.Request.URL.Path
	page := c.Query("page")
	pl := c.Query("pl")
	nav := getNavigation(page)

	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	{
		playlistID := spotify.ID(pl)
		options := new(spotify.Options)
		session := sessions.Default(c)
		land := session.Get("country")
		if land != nil {
			country := land.(string)
			options.Country = &country
		}
		offset, _ := strconv.Atoi(page)
		offset = offset * pageLimit
		options.Offset = &offset
		limit := pageLimit
		options.Limit = &limit
		fields := "items.track(id,name,album(name,images),artists)"
		plTracks, err := spotifyClient.GetPlaylistTracksOpt(playlistID, options, fields)
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var tracks []topTrack
		for {
			for _, item := range plTracks.Tracks {
				var tt topTrack
				if item.Track.ID == "" {
					continue
				}
				tt.Name = item.Track.Name
				tt.Album = item.Track.Album.Name
				tt.Artists = joinArtists(item.Track.Artists, ", ")
				tt.URL = item.Track.ExternalURLs["spotify"]
				tt.Image = item.Track.Album.Images[0].URL
				tracks = append(tracks, tt)
			}
			err = spotifyClient.NextPage(plTracks)
			if err == spotify.ErrNoMorePages {
				break
			}
			if err != nil {
				log.Println(err.Error())
			}
		}
		var pls frontendAlbumPlaylist
		plist, err := spotifyClient.GetPlaylist(playlistID)
		pls.ID = plist.ID.String()
		pls.Name = plist.Name
		pls.Owner = plist.Owner.DisplayName
		pls.URL = plist.ExternalURLs["spotify"]
		pls.Image = plist.Images[0].URL
		pls.Tracks = plist.Tracks.Total

		nav.Back = "/playlists"
		nav.Endpoint = endpoint
		c.HTML(
			http.StatusOK,
			"playlistTracks.html",
			gin.H{
				"Tracks":     tracks,
				"Playlist":   pls,
				"Navigation": nav,
				"title":      "Tracks",
			},
		)
	}
}

/* playlists - display some of user's playlists
 */
func playlists(c *gin.Context) {
	endpoint := c.Request.URL.Path
	page := c.Query("page")
	nav := getNavigation(page)

	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	{
		options := new(spotify.Options)
		offset, _ := strconv.Atoi(page)
		offset = offset * pageLimit
		options.Offset = &offset
		limit := pageLimit
		options.Limit = &limit
		pages, err := spotifyClient.CurrentUsersPlaylistsOpt(options)
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var pls []frontendAlbumPlaylist
		for _, item := range pages.Playlists {
			var pl frontendAlbumPlaylist
			pl.ID = item.ID.String()
			pl.Name = item.Name
			pl.Owner = item.Owner.DisplayName
			pl.URL = item.ExternalURLs["spotify"]
			pl.Image = item.Images[0].URL
			pl.Tracks = int(item.Tracks.Total)
			pls = append(pls, pl)
		}
		nav.Endpoint = endpoint
		c.HTML(
			http.StatusOK,
			"playlists.html",
			gin.H{
				"title":      "Playlists",
				"Navigation": nav,
				"Playlists":  pls,
			},
		)
	}
}

func albumTracks(c *gin.Context) {
	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	defer func() {
		al := c.Query("al")
		albumID := spotify.ID(al)
		// use the client to make calls that require authorization
		alTracks, err := spotifyClient.GetAlbumTracks(albumID)
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var tracks []topTrack
		for {
			for _, item := range alTracks.Tracks {
				if item.ID == "" {
					continue
				}
				var tt topTrack
				tt.Name = item.Name
				tt.Artists = joinArtists(item.Artists, ", ")
				tt.URL = item.ExternalURLs["spotify"]
				tracks = append(tracks, tt)
			}
			err = spotifyClient.NextPage(alTracks)
			if err == spotify.ErrNoMorePages {
				break
			}
			if err != nil {
				log.Println(err.Error())
			}
		}
		var alb frontendAlbumPlaylist
		album, err := spotifyClient.GetAlbum(albumID)
		alb.ID = album.ID.String()
		alb.Name = album.Name
		alb.URL = album.ExternalURLs["spotify"]
		alb.Image = album.Images[0].URL
		alb.Tracks = album.Tracks.Total
		c.HTML(
			http.StatusOK,
			"albumTracks.html",
			gin.H{
				"Tracks": tracks,
				"Album":  alb,
				"title":  "Tracks",
			},
		)
	}()
}

/* albums - display some of user's albums
 */
func albums(c *gin.Context) {
	endpoint := c.Request.URL.Path
	page := c.Query("page")
	nav := getNavigation(page)

	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	{
		options := new(spotify.Options)
		session := sessions.Default(c)
		land := session.Get("country")
		if land != nil {
			country := land.(string)
			options.Country = &country
		}
		offset, _ := strconv.Atoi(page)
		offset = offset * pageLimit
		options.Offset = &offset
		limit := pageLimit
		options.Limit = &limit

		userAlbums, err := spotifyClient.CurrentUsersAlbumsOpt(options)
		if err != nil {
			log.Panic(err)
			c.String(http.StatusNotFound, err.Error())
		}
		var als []frontendAlbumPlaylist
		for _, item := range userAlbums.Albums {
			var al frontendAlbumPlaylist
			al.ID = item.ID.String()
			al.Name = item.Name
			al.Artists = joinArtists(item.Artists, ", ")
			al.URL = item.ExternalURLs["spotify"]
			al.Image = item.Images[0].URL
			al.Tracks = item.Tracks.Total
			als = append(als, al)
		}
		nav.Back = "/playlists"
		nav.Endpoint = endpoint
		c.HTML(
			http.StatusOK,
			"albums.html",
			gin.H{
				"title":      "Albums",
				"Navigation": nav,
				"Albums":     als,
			},
		)
	}
}

/* -------- TODO - everything below has to change or go -------- */

/* artists - displays user followed artists
 */
func artists(c *gin.Context) {
	spotifyClient := clientMagic(c)
	if spotifyClient == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	defer func() {
		// use the client to make calls that require authorization
		artists, err := spotifyClient.CurrentUsersFollowedArtists()
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
TODO - make proper search using all options
*/
func search(c *gin.Context) {
	qCookie, cookieErr := c.Cookie("search_query")
	cCookie, _ := c.Cookie("search_category")
	endpoint := c.Request.URL.Path
	if cookieErr != nil {
		qCookie = "NotSet"
		cCookie = "NotSet"
		c.SetCookie("search_query", c.Query("q"), cookieLifetime, endpoint, "", false, true)
		c.SetCookie("search_category", c.Query("c"), cookieLifetime, endpoint, "", false, true)
	}
	log.Printf("Cookie values: %s %s \n", qCookie, cCookie)
	client := clientMagic(c)
	if client == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
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

/* recommend songs based on given tracks (maximum 7)
accepts query parameters t3..t5 with trackIDs.
prints recommended tracks
*/
func recommend(c *gin.Context) {
	endpoint := c.Request.URL.Path
	for i := 3; i < 6; i++ {
		cookieName := fmt.Sprintf("t%d", i)
		c.SetCookie(cookieName, c.Query(cookieName), 47, endpoint, "", false, true)
		log.Printf("Cookie %s value: %s \n", cookieName, c.Query(cookieName))
	}
	client := clientMagic(c)
	if client == nil {
		c.JSON(http.StatusTeapot, gin.H{"message": "failed to find  client"})
		return
	}

	defer func() {
		trackIDs := []spotify.ID{}
		for i := 3; i < 6; i++ {
			cookieName := fmt.Sprintf("t%d", i)
			if track, err := c.Cookie(cookieName); err == nil {
				if len(track) > 2 {
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
