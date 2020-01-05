package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
)

type songListenedAt struct {
	Track   spotify.SimpleTrack
	AddedAt string
}

type songData struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Date      string `json:"date"`
	Timestamp int    `json:"timestamp"`
	Reducer   string `json:"reducer"`
}

type recommendationParameters struct {
	Seeds           spotify.Seeds
	TrackAttributes *spotify.TrackAttributes
	FromYear        int
	MinTrackCount   int
}

func clientMagic(c *gin.Context) *spotify.Client {
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
			return nil
		}
	}
	return client
}

/* getClient - restore client for given endpoint/state from cache
or return nil
*/
func getClient(endpoint string) *spotify.Client {
	if gclient, foundClient := kaszka.Get(endpoint); foundClient {
		log.Printf("getClient: Cached client found for: %s", endpoint)
		client := gclient.(*spotify.Client)
		if tok, err := client.Token(); err != nil {
			log.Panic(err)
		} else {
			log.Printf("getClient: Token will expire in %s", tok.Expiry.Sub(time.Now()).String())
		}
		return client
	}
	log.Printf("getClient: No cached client found for: %s", endpoint)
	if storeToken[endpoint] { // token is only stored in database for few endpoints
		tok, err := getTokenFromDB(endpoint)
		if err == nil {
			client := auth.NewClient(tok)
			kaszka.Set(endpoint, &client, tok.Expiry.Sub(time.Now()))
			log.Printf("getClient: Cached client for: %s", endpoint)
			if storeToken[endpoint] {
				if m, _ := time.ParseDuration("5m30s"); time.Until(tok.Expiry) < m {
					newToken, _ := client.Token()
					updateTokenInDB(endpoint, newToken)
				}
			}
			return &client
		}
	}
	return nil
}

func getTokenFromDB(endpoint string) (*oauth2.Token, error) {
	tokenID := strings.TrimPrefix(endpoint, "/")
	dsnap, err := firestoreClient.Collection("tokens").Doc(tokenID).Get(ctx)
	if err != nil {
		log.Printf("Error retrieving token from Firestore for endpoint %s %s.\nPossibly it ain't there..", endpoint, err.Error())
		return nil, err
	}
	tok := &oauth2.Token{}
	dsnap.DataTo(tok)
	log.Printf("getTokenFromDB: Got token with expiration %s", tok.Expiry.In(location).Format("15:04:05"))
	return tok, nil
}

func saveTokenToDB(endpoint string, token *oauth2.Token) {
	tokenID := strings.TrimPrefix(endpoint, "/")
	_, err := firestoreClient.Collection("tokens").Doc(tokenID).Set(ctx, token)

	if err != nil {
		log.Printf("saveToken: Error saving token for endpoint %s %s", endpoint, err.Error())
	} else {
		log.Printf("saveToken: Saved token for endpoint %s into Firestore", endpoint)
		log.Printf("saveToken: Token expiration %s", token.Expiry.In(location).Format("15:04:05"))
	}
}

func updateTokenInDB(endpoint string, token *oauth2.Token) {
	tokenID := strings.TrimPrefix(endpoint, "/")
	_, err := firestoreClient.Collection("tokens").Doc(tokenID).Set(ctx, map[string]interface{}{
		"AccessToken":  token.AccessToken,
		"Expiry":       token.Expiry,
		"RefreshToken": token.RefreshToken,
		"TokenType":    token.TokenType,
	}, firestore.MergeAll)

	if err != nil {
		log.Printf("updateToken: Error saving token for endpoint %s %s", endpoint, err.Error())
	} else {
		log.Printf("updateToken: Saved token for endpoint %s into Firestore", endpoint)
		log.Printf("updateToken: Token expiration %s", token.Expiry.In(location).Format("15:04:05"))
	}
}

/* recommendFromHistory - (like recommendFromMood but latest tracks
are taken from Firebase store (unique tracks etc.)
TODO - add limit as parameter (short, medium, long)
*/
func recommendFromHistory(client *spotify.Client) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracksIDs := []spotify.ID{}
	iter := firestoreClient.Collection("recently_played").OrderBy("played_at", firestore.Desc).Limit(24).Documents(ctx)
	var tr firestoreTrack
	// fiil in recentTracksIDs
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		// log.Println(doc.Ref.ID) get TrackIDs, must be after iterator.Done otherwise we hit nil pointer
		if err != nil {
			log.Println(err.Error())
			return recommendedTracks, err
		}
		recentTracksIDs = append(recentTracksIDs, spotify.ID(doc.Ref.ID))
		if err := doc.DataTo(&tr); err != nil {
			log.Println(err.Error())
			return recommendedTracks, err
		}
	}
	// get full tracks for track IDs
	recentTracks, err := fullTrackGetMany(client, recentTracksIDs)
	if err != nil {
		return recommendedTracks, err
	}
	// get attributes for tracks
	trackAttributes, err := getTrackAttributes(client, recentTracks)
	if err != nil {
		return recommendedTracks, err
	}
	// modern tracks not oldies, seed by recent 5 tracks and average attributes of retrieved tracks (24)
	params := recommendationParameters{
		FromYear:      1999,
		MinTrackCount: 20,
		Seeds: spotify.Seeds{
			Tracks: recentTracksIDs[0:4],
		},
		TrackAttributes: trackAttributes,
	}

	pageTracks, err := getRecommendedTracks(client, params)
	if err != nil {
		return recommendedTracks, err
	}
	recommendedTracks = append(recommendedTracks, pageTracks...)

	return recommendedTracks, nil
}

/* recommendFromMood - suggest new music based on recently playing tracks
5 recent tracks and averaged attibutes of of recent tracks
*/
func recommendFromMood(client *spotify.Client) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracksIDs := []spotify.ID{}

	recentlyPlayed, err := client.PlayerRecentlyPlayed()
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to get user's recently played: %v", err)
	}

	for _, item := range recentlyPlayed {
		recentTracksIDs = appendIfUnique(recentTracksIDs, item.Track.ID)
	}
	recentTracks, errF := fullTrackGetMany(client, recentTracksIDs)
	if errF != nil {
		return recommendedTracks, errF
	}
	trackAttributes, errA := getTrackAttributes(client, recentTracks)
	if errA != nil {
		return recommendedTracks, errA
	}

	params := recommendationParameters{
		FromYear:      1999,
		MinTrackCount: 20,
		Seeds: spotify.Seeds{
			Tracks: recentTracksIDs[0:4],
		},
		TrackAttributes: trackAttributes,
	}

	pageTracks, err := getRecommendedTracks(client, params)
	if err != nil {
		return recommendedTracks, err
	}

	recommendedTracks = append(recommendedTracks, pageTracks...)

	return recommendedTracks, nil
}

/* recommendFromTop - recommend music based on your top artists and
averaged attributes of user's top tracks
*/
func recommendFromTop(client *spotify.Client) ([]spotify.FullTrack, error) {
	tracks := []spotify.FullTrack{}
	limit := 5
	userTopArtists, err := client.CurrentUsersTopArtistsOpt(&spotify.Options{Limit: &limit})
	if err != nil {
		return tracks, fmt.Errorf("Failed to get user's top artists: %v", err)
	}

	userTopTracks, err := client.CurrentUsersTopTracks()
	if err != nil {
		return tracks, fmt.Errorf("Failed to get user's top tracks: %v", err)
	}

	trackAttributes, err := getTrackAttributes(client, userTopTracks.Tracks)
	if err != nil {
		return tracks, err
	}

	for _, artist := range userTopArtists.Artists {
		log.Printf("Fetching recommendations seeded by artist %s", artist.Name)

		params := recommendationParameters{
			FromYear:      1999,
			MinTrackCount: pageLimit,
			Seeds: spotify.Seeds{
				Artists: []spotify.ID{artist.ID},
			},
			TrackAttributes: trackAttributes,
		}

		pageTracks, err := getRecommendedTracks(client, params)
		if err != nil {
			return tracks, err
		}

		log.Printf("Fetched %d recommendations seeded by artist %s", len(pageTracks), artist.Name)

		tracks = append(tracks, pageTracks...)
	}

	return tracks, nil
}

func miniAudioFeatures(ids []spotify.ID, client *spotify.Client) *[]audioTrack {
	var f audioTrack
	var audioTracks []audioTrack
	chunks := chunkIDs(ids, pageLimit)
	audioFeatures, _ := client.GetAudioFeatures(chunks[0]...) // GetAudioFeatures has variadic argument
	fullTracks, _ := fullTrackGetMany(client, chunks[0])
	for i, res := range audioFeatures {
		if res.ID != fullTracks[i].ID {
			log.Println("miniAudioFeatures: NOT IN SYNC")
		}
		f.ID = res.ID
		f.Name = fullTracks[i].Name
		f.Artists = joinArtists(fullTracks[i].Artists, ", ")
		f.Energy = int(100.0 * res.Energy)
		f.Loudness = int(-1.66 * res.Loudness)
		f.Tempo = int(res.Tempo - 100.0)
		f.Instrumentalness = int(100.0 * res.Instrumentalness)
		f.Acousticness = int(100.0 * res.Acousticness)
		f.URL = fullTracks[i].ExternalURLs["spotify"]
		audioTracks = append(audioTracks, f)
	}
	return &audioTracks
}

/* getTrackAttributes - return averaged attributes for set of tracks
 */
func getTrackAttributes(client *spotify.Client, tracks []spotify.FullTrack) (*spotify.TrackAttributes, error) {
	var attributes *spotify.TrackAttributes

	features, err := client.GetAudioFeatures(getSpotifyIDs(tracks)...)
	if err != nil {
		return attributes, fmt.Errorf(
			"Failed to get audio features of %d track(s): %v",
			len(tracks),
			err,
		)
	}

	acousticness := []float64{}
	instrumentalness := []float64{}
	liveness := []float64{}
	energy := []float64{}
	valence := []float64{}

	for _, feature := range features {
		acousticness = append(acousticness, float64(feature.Acousticness))
		instrumentalness = append(instrumentalness, float64(feature.Instrumentalness))
		liveness = append(liveness, float64(feature.Liveness))
		energy = append(energy, float64(feature.Energy))
		valence = append(valence, float64(feature.Valence))
	}

	averageAcousticness := averageFloat(acousticness)
	averageInstrumentalness := averageFloat(instrumentalness)
	averageLiveness := averageFloat(liveness)
	averageEnergy := averageFloat(energy)
	averageValence := averageFloat(valence)

	attributes = spotify.NewTrackAttributes().
		MaxAcousticness(asAttribute("max", averageAcousticness)).
		MinAcousticness(asAttribute("min", averageAcousticness)).
		MaxEnergy(asAttribute("max", averageEnergy)).
		MinEnergy(asAttribute("min", averageEnergy)).
		MaxInstrumentalness(asAttribute("max", averageInstrumentalness)).
		MinInstrumentalness(asAttribute("min", averageInstrumentalness)).
		MaxLiveness(asAttribute("max", averageLiveness)).
		MinLiveness(asAttribute("min", averageLiveness)).
		MaxValence(asAttribute("max", averageValence)).
		MinValence(asAttribute("min", averageValence))

	return attributes, nil
}

/* handleSearchResults - pretty print search results depending
on search category (tracks, playlists, albums)
*/
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
