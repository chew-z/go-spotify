package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/zmb3/spotify"
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

/* getClient - restore client for given endpoint/state from cache
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

/* recommendFromHistory - (like recommendFromMood but latest tracks
are taken from Firebase store (unique tracks etc.)
TODO - add limit as parameter (short, medium, long)
*/
func recommendFromHistory(client *spotify.Client) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracksIDs := []spotify.ID{}
	ctx := context.Background()
	firestoreClient := initFirestoreDatabase(ctx)
	defer firestoreClient.Close()
	iter := firestoreClient.Collection("recently_played").OrderBy("played_at", firestore.Desc).Limit(24).Documents(ctx)
	var tr firestoreTrack
	// fiil in recentTracksIDs
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

func recommendFromTop(client *spotify.Client) ([]spotify.FullTrack, error) {
	tracks := []spotify.FullTrack{}
	pageLimit := 5
	userTopArtists, err := client.CurrentUsersTopArtistsOpt(&spotify.Options{Limit: &pageLimit})
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
			MinTrackCount: 100,
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
