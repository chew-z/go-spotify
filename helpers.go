package main

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zmb3/spotify"
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

var (
	reducerPastPlaylistID = spotify.ID("7MHn8B6AcI0SK6qFfvcrHL")
	bufferPlaylistID      = spotify.ID("1rWKf36NvH4q6imCstXvy4")
	playlistsToMonitor    = []spotify.ID{
		bufferPlaylistID,
	}
)

type recommendationParameters struct {
	Seeds           spotify.Seeds
	TrackAttributes *spotify.TrackAttributes
	FromYear        int
	MinTrackCount   int
}

func appendIfMissing(slice []spotify.ID, i spotify.ID) []spotify.ID {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}
func recommendFromMood(client *spotify.Client) ([]spotify.FullTrack, error) {
	recommendedTracks := []spotify.FullTrack{}
	recentTracks := []spotify.FullTrack{}
	recentArtistsIDs := []spotify.ID{}
	recentTracksIDs := []spotify.ID{}

	recentlyPlayed, err := client.PlayerRecentlyPlayed()
	if err != nil {
		return recommendedTracks, fmt.Errorf("Failed to get user's recently played: %v", err)
	}

	for _, item := range recentlyPlayed {
		track, _ := client.GetTrack(item.Track.ID)
		recentTracks = append(recentTracks, *track)
		recentArtistsIDs = appendIfMissing(recentArtistsIDs, item.Track.Artists[0].ID)
		recentTracksIDs = appendIfMissing(recentTracksIDs, item.Track.ID)
	}
	trackAttributes, err := getTrackAttributes(client, recentTracks)
	if err != nil {
		return recommendedTracks, err
	}

	params := recommendationParameters{
		FromYear:      1999,
		MinTrackCount: 20,
		Seeds: spotify.Seeds{
			// Artists: recentArtistsIDs[0:4],
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

func getRecommendedTracks(client *spotify.Client, params recommendationParameters) ([]spotify.FullTrack, error) {
	pageLimit := 100
	trackCount := 0
	totalCount := 0
	tracks := []spotify.FullTrack{}

	options := spotify.Options{
		Limit:   &pageLimit,
		Offset:  &totalCount,
		Country: &countryPoland,
	}

	page, err := client.GetRecommendations(params.Seeds, params.TrackAttributes, &options)
	if err != nil {
		return tracks, fmt.Errorf("Failed to get recommendations: %v", err)
	}

	totalCount += len(page.Tracks)

	fullTracks, err := fullTrackGetMany(client, getSpotifyIDs(page.Tracks))
	if err != nil {
		return tracks, err
	}

	for _, track := range fullTracks {
		album, err := fullAlbumGet(client, track.Album.ID)
		if err != nil {
			return tracks, err
		}

		if album.ReleaseDateTime().Year() >= params.FromYear {
			trackCount++
			tracks = append(tracks, track)
		}
	}

	return tracks, nil
}

func fullTrackGetMany(client *spotify.Client, ids []spotify.ID) ([]spotify.FullTrack, error) {
	pageLimit := 50
	tracks := []spotify.FullTrack{}

	if len(ids) == 0 {
		return tracks, nil
	}

	chunks := chunkIDs(ids, pageLimit)

	for _, chunkIDs := range chunks {
		pointerTracks, err := client.GetTracks(chunkIDs...)
		if err != nil {
			return tracks, fmt.Errorf("Failed to get many tracks: %v", err)
		}

		for _, track := range pointerTracks {
			tracks = append(tracks, *track)
		}
	}

	return tracks, nil
}

func chunkIDs(ids []spotify.ID, chunkSize int) [][]spotify.ID {
	chunks := [][]spotify.ID{[]spotify.ID{}}

	for _, id := range ids {
		chunkIndex := len(chunks) - 1

		if len(chunks[chunkIndex]) < chunkSize {
			chunks[chunkIndex] = append(chunks[chunkIndex], id)
		} else {
			chunks = append(chunks, []spotify.ID{id})
		}
	}

	return chunks
}

func fullAlbumGet(client *spotify.Client, id spotify.ID) (spotify.FullAlbum, error) {
	var albumCache = map[spotify.ID]spotify.FullAlbum{}
	if album, exists := albumCache[id]; exists {
		return album, nil
	}

	album, err := client.GetAlbum(id)
	if err != nil {
		return spotify.FullAlbum{}, fmt.Errorf("Failed to get full album %s: %v", id, err)
	}

	albumCache[id] = *album

	return *album, nil
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

func asAttribute(attributeType string, value float64) float64 {
	minValue := 0.0
	maxValue := 1.0
	modifier := 0.3

	switch strings.ToLower(attributeType) {
	case "max":
		minValue = 0.3

		break
	case "min":
		maxValue = .8
		modifier = -modifier

		break
	default:
		log.Printf("Received an invalid recommendation attributeType: %s", attributeType)

		break
	}

	if value < .5 {
		return math.Max(value+modifier, minValue)
	}

	return math.Min(value+modifier, maxValue)
}

func getSpotifyIDs(input interface{}) []spotify.ID {
	values := getItemPropertyValue(input, "ID")
	ids := []spotify.ID{}

	for _, value := range values {
		if id, ok := value.(spotify.ID); ok {
			ids = append(ids, id)
		} else {
			panic(&reflect.ValueError{Method: "GetSpotifyIDs", Kind: reflect.ValueOf(value).Kind()})
		}
	}

	return ids
}

func getItemPropertyValue(input interface{}, fieldName string) []interface{} {
	var slice reflect.Value
	output := []interface{}{}

	value := reflect.ValueOf(input)

	// Support both pointers and slices
	if value.Kind() == reflect.Ptr {
		slice = value.Elem()
	} else {
		slice = value
	}

	for i := 0; i < slice.Len(); i++ {
		fieldValue := slice.Index(i).FieldByName(fieldName)

		output = append(output, fieldValue.Interface())
	}

	return output
}

func averageFloat(values []float64) float64 {
	var total float64

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func refreshReducer(client *spotify.Client) error {
	currentTime := time.Now()
	// get tracks to add
	tracksToAdd := []songListenedAt{}
	for _, playlistID := range playlistsToMonitor {
		trackNum, err := client.GetPlaylist(playlistID)
		if err != nil {
			log.Println(err)
		}
		numTracks := trackNum.Tracks.Total
		for i := 0; i < numTracks; i += 100 {
			tracksPage, err := client.GetPlaylistTracksOpt(spotify.ID(playlistID), &spotify.Options{Offset: &i}, "")
			for _, track := range tracksPage.Tracks {
				tracksToAdd = append(tracksToAdd, songListenedAt{track.Track.SimpleTrack, track.AddedAt})
			}
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}

	recentlyPlayed, err := client.PlayerRecentlyPlayed()

	if err != nil {
		println(err)
	}

	for _, track := range recentlyPlayed {
		tracksToAdd = append(tracksToAdd, songListenedAt{track.Track, track.PlayedAt.Format(spotify.TimestampLayout)})
	}
	tracksAddedCount := 0

	// add tracks to dynamo / spotify
	for _, track := range tracksToAdd {
		addedDate, _ := time.Parse(spotify.TimestampLayout, track.AddedAt)
		startOfDay := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, currentTime.Location())
		if addedDate.Before(startOfDay) {
			continue
		}

		reducerPastTracks, err := client.GetPlaylistTracks(reducerPastPlaylistID)
		if err != nil {
			return err
		}

		for _, pt := range reducerPastTracks.Tracks {
			if pt.Track.ID == track.Track.ID {
				continue
			}
		}

		tracksAddedCount++

		if err != nil {
			return err
		}

	}

	var tracksAddedIDs []spotify.ID
	for _, track := range tracksToAdd {
		tracksAddedIDs = append(tracksAddedIDs, track.Track.ID)
	}

	client.AddTracksToPlaylist(reducerPastPlaylistID, tracksAddedIDs...)
	client.RemoveTracksFromPlaylist(bufferPlaylistID, tracksAddedIDs...)

	log.Printf("%d tracks added\n", tracksAddedCount)

	return nil
}

/* ------------------- */

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
