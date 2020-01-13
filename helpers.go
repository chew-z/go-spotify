package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"cloud.google.com/go/firestore"
	spotify "github.com/chew-z/spotify"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

type recommendationParameters struct {
	Seeds           spotify.Seeds
	TrackAttributes *spotify.TrackAttributes
	FromYear        int
	MinTrackCount   int
}

/* recommendFromHistory - (like recommendFromMood but latest tracks
are taken from Firebase store (unique tracks etc.)
TODO - add limit as parameter (short, medium, long)
*/
func recommendFromHistory(spotifyClient *spotify.Client, c *gin.Context) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracksIDs := []spotify.ID{}
	session := sessions.Default(c)
	user := session.Get("user").(string)
	country := session.Get("country").(string)
	//  get latest [pageLimit] tracks from firestore
	path := fmt.Sprintf("users/%s/recently_played", user)
	iter := firestoreClient.Collection(path).OrderBy("played_at", firestore.Desc).Limit(pageLimit).Documents(ctx)
	// var tr firestoreTrack
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
		recentTracksIDs = append(recentTracksIDs, spotify.ID(doc.Ref.ID)) // TODO == *tr.ID
		// if err := doc.DataTo(&tr); err != nil {
		// 	log.Println(err.Error())
		// 	return recommendedTracks, err
		// }
	}
	// get full tracks for track IDs
	recentTracks, err := fullTrackGetMany(spotifyClient, recentTracksIDs)
	if err != nil {
		return recommendedTracks, err
	}
	// get attributes for tracks
	trackAttributes, err := getTrackAttributes(spotifyClient, recentTracks)
	if err != nil {
		return recommendedTracks, err
	}
	// modern tracks not oldies, seed by recent 5 tracks and average attributes of retrieved tracks (pageLimit)
	params := recommendationParameters{
		FromYear:      1999,
		MinTrackCount: 20,
		Seeds: spotify.Seeds{
			Tracks: recentTracksIDs[0:4],
		},
		TrackAttributes: trackAttributes,
	}
	// get recommendations
	pageTracks, err := getRecommendedTracks(spotifyClient, params, &country)
	if err != nil {
		return recommendedTracks, err
	}
	// TODO save on this call we do not loop
	// recommendedTracks = append(recommendedTracks, pageTracks...)

	return pageTracks, nil
}

/* recommendFromMood - suggest new music based on recently playing tracks
5 recent tracks and averaged attibutes of of recent tracks.
It works well.
TODO - add parametrization
*/
func recommendFromMood(spotifyClient *spotify.Client) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracksIDs := []spotify.ID{}
	// get recently played tracks
	recentlyPlayed, err := spotifyClient.PlayerRecentlyPlayed()
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to get user's recently played: %v", err)
	}
	// but only unique no hiccups
	for _, item := range recentlyPlayed {
		recentTracksIDs = appendIfUnique(recentTracksIDs, item.Track.ID)
	}
	// get full tracks
	recentTracks, err := fullTrackGetMany(spotifyClient, recentTracksIDs)
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to full tracks: %v", err)
	}
	// get averaged attributes
	trackAttributes, err := getTrackAttributes(spotifyClient, recentTracks)
	if err != nil {
		return recommendedTracks, err
	}
	// seed with recent five tracks and attributes of recently played
	params := recommendationParameters{
		FromYear:      1999,
		MinTrackCount: 20,
		Seeds: spotify.Seeds{
			Tracks: recentTracksIDs[0:4],
		},
		TrackAttributes: trackAttributes,
	}
	user, err := spotifyClient.CurrentUser() //TODO - all this isn't necessary
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to get user: %v", err)
	}
	country := string(user.Country)
	// get recommendation
	pageTracks, err := getRecommendedTracks(spotifyClient, params, &country)
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to get recommended tracks: %v", err)
	}
	// TODO This is not necessary as we are not looping
	// recommendedTracks = append(recommendedTracks, pageTracks...)

	return pageTracks, nil
}

/* recommendFromTop - recommend music based on your top artists and
averaged attributes of user's top tracks
TODO - this doesn't make sense like getting country tracks for Miles Davis
*/
func recommendFromTop(spotifyClient *spotify.Client) ([]spotify.FullTrack, error) {
	tracks := []spotify.FullTrack{}
	limit := 5
	// Get top five artists
	userTopArtists, err := spotifyClient.CurrentUsersTopArtistsOpt(&spotify.Options{Limit: &limit})
	if err != nil {
		return tracks, fmt.Errorf("Failed to get user's top artists: %v", err)
	}
	// get top tracks
	userTopTracks, err := spotifyClient.CurrentUsersTopTracks()
	if err != nil {
		return tracks, fmt.Errorf("Failed to get user's top tracks: %v", err)
	}
	// get averaged attributes (audio features) for top tracks
	trackAttributes, err := getTrackAttributes(spotifyClient, userTopTracks.Tracks)
	if err != nil {
		return tracks, err
	}
	user, err := spotifyClient.CurrentUser() //TODO - all this isn't necessary
	if err != nil {
		return tracks, fmt.Errorf("Failed to get user: %v", err)
	}
	country := string(user.Country)
	// Loop over top artists and get recommendations
	// This doesn't make any sense to me with diverse tastes
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
		pageTracks, err := getRecommendedTracks(spotifyClient, params, &country)
		if err != nil {
			return tracks, err
		}

		log.Printf("Fetched %d recommendations seeded by artist %s", len(pageTracks), artist.Name)

		tracks = append(tracks, pageTracks...)
	}

	return tracks, nil
}

/* miniAudioFeatures - quickly implemented function to feed charts
gets selected audio features for songs, normalizes them (TODO - think through)
and packs and returns
*/
func miniAudioFeatures(ids []spotify.ID, spotifyClient *spotify.Client) *[]audioTrack {
	var f audioTrack
	var audioTracks []audioTrack
	chunks := chunkIDs(ids, pageLimit)
	audioFeatures, _ := spotifyClient.GetAudioFeatures(chunks[0]...) // GetAudioFeatures has variadic argument
	fullTracks, _ := fullTrackGetMany(spotifyClient, chunks[0])
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
		f.Image = fullTracks[i].Album.Images[2].URL
		audioTracks = append(audioTracks, f)
	}
	return &audioTracks
}

/* getTrackAttributes - return averaged attributes for set of tracks
 */
func getTrackAttributes(spotifyClient *spotify.Client, tracks []spotify.FullTrack) (*spotify.TrackAttributes, error) {
	var attributes *spotify.TrackAttributes

	features, err := spotifyClient.GetAudioFeatures(getSpotifyIDs(tracks)...)
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
