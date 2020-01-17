package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"

	spotify "github.com/chew-z/spotify"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

type timeZones struct {
	Time string   `json:"time"`
	Zone []string `json:"zone"`
}

/*getTime fetches the contents of the given URL and decodes it as JSON
into the given result, which should be a pointer to the expected data.
*/
func getTime(url string) (*timeZones, error) {
	var result timeZones
	if gae != "" || gcr != "" {
		token := getJWToken(timezonesURL)
		if token != "" {
			if verifyToken(timezonesURL, token) {
				req, err := http.NewRequest("GET", url, nil)
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
				req.Header.Add("content-type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return nil, err
				}
				err = json.NewDecoder(resp.Body).Decode(&result)
				if err != nil {
					return nil, fmt.Errorf("cannot decode JSON: %v", err)
				}
			}
		}
	}
	return &result, nil
}

/*getUserLocation - on AppEngine it is getting City, lat/lon
from AppEngine-specific request headers and using microservice
to get timezone from lat/lon
*/
func getUserLocation(c *gin.Context) *userLocation {
	var loc userLocation

	loc.City = strings.Title(c.Request.Header.Get("X-AppEngine-City"))
	if loc.City == "?" {
		loc.City = ""
	}
	latlon := c.Request.Header.Get("X-AppEngine-CityLatLong")
	if latlon != "" {
		ll := strings.Split(latlon, ",")
		loc.Lat = ll[0]
		loc.Lon = ll[1]
	}
	url := fmt.Sprintf("%s?lat=%s&lon=%s", timezonesURL, loc.Lat, loc.Lon)
	tzResponse, err := getTime(url)
	if err == nil {
		loc.Tz = tzResponse.Zone[0]
		loc.Time = tzResponse.Time
	}
	return &loc
}

/*getRecommendedTracks - gets recommendation based on seed
 */
func getRecommendedTracks(spotifyClient *spotify.Client, params recommendationParameters, country *string) ([]spotify.FullTrack, error) {
	limit := pageLimit
	tracks := []spotify.FullTrack{}
	options := spotify.Options{
		Limit:   &limit,
		Country: country, // TODO - this and location
	}
	// get recommendtions (only single page)
	page, err := spotifyClient.GetRecommendations(params.Seeds, params.TrackAttributes, &options)
	if err != nil {
		return tracks, fmt.Errorf("Failed to get recommendations: %v", err)
	}
	// TODO - we might skip both logic before to speed up
	// all this is only necessary to limit release date
	fullTracks, err := fullTrackGetMany(spotifyClient, getSpotifyIDs(page.Tracks))
	if err != nil {
		return tracks, err
	}
	// for _, track := range fullTracks {
	// 	album, err := fullAlbumGet(client, track.Album.ID)
	// 	if err != nil {
	// 		return tracks, err
	// 	}
	// 	if album.ReleaseDateTime().Year() >= params.FromYear {
	// 		tracks = append(tracks, track)
	// 	}
	// }
	// return tracks, nil
	return fullTracks, nil
}

/*fullTracksGetMany - gets FullTrack objects for given track IDs
This is used a lot and pehaps could be speed up
*/
func fullTrackGetMany(spotifyClient *spotify.Client, ids []spotify.ID) ([]spotify.FullTrack, error) {
	tracks := []spotify.FullTrack{}

	if len(ids) == 0 {
		return tracks, nil
	}
	// get results in chunks
	// TODO we are using single chunk for now
	chunks := chunkIDs(ids, pageLimit)
	for _, chunkIDs := range chunks {
		pointerTracks, err := spotifyClient.GetTracks(chunkIDs...)
		if err != nil {
			return tracks, fmt.Errorf("Failed to get many tracks: %v", err)
		}
		for _, track := range pointerTracks {
			tracks = append(tracks, *track)
		}
	}
	return tracks, nil
}

func fullAlbumGet(spotifyClient *spotify.Client, id spotify.ID) (spotify.FullAlbum, error) {
	// var albumCache = map[spotify.ID]spotify.FullAlbum{}
	// if album, exists := albumCache[id]; exists {
	// 	return album, nil
	// }
	album, err := spotifyClient.GetAlbum(id)
	if err != nil {
		return spotify.FullAlbum{}, fmt.Errorf("Failed to get full album %s: %v", id, err)
	}
	// albumCache[id] = *album
	return *album, nil
}

/*chunkIDs - split large vector of spotify IDs into chunks
 */
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

func asAttribute(attributeType string, value float64) float64 {
	minValue := 0.0
	maxValue := 1.0
	modifier := 0.3

	switch strings.ToLower(attributeType) {
	case "max":
		minValue = .3

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

/*getTimezones - returns timezones for the country
we keep collection 'timezones' just for that
*/
func getTimeZones(country string) ([]string, error) {
	type timeZone struct {
		Country  string
		Timezone string
	}
	var tz timeZone
	var timezones []string
	iter := firestoreClient.Collection("timezones").Where("Country", "==", country).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		doc.DataTo(&tz)
		timezones = append(timezones, tz.Timezone)
	}
	return timezones, nil
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

/* jointArtists - merge all artists names into single string
 */
func joinArtists(artists []spotify.SimpleArtist, separator string) string {
	return strings.Join(
		func() []string {
			output := []string{}
			for _, a := range artists {
				output = append(output, a.Name)
			}
			return output
		}(),
		separator,
	)
}

/*appendIfUnique - add spotify ID to vector only if it is unique
 */
func appendIfUnique(slice []spotify.ID, i spotify.ID) []spotify.ID {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

/* normalizeRecentlyPlayed - my Spotify history has hiccups
due to poor connection and switching between different players (Chromecast audio)
so if two tracks are identical and started within 30 seconds take only one
Spotify should do that (count at least 30 secs as played track) and theoretically does
 but its not
*/
func normalizeRecentlyPlayed(incoming []spotify.RecentlyPlayedItem) []spotify.RecentlyPlayedItem {
	var outgoing []spotify.RecentlyPlayedItem
	outgoing = append(outgoing, incoming[0])
	for i := 1; i < len(incoming); i++ {
		t1 := incoming[i-1].PlayedAt
		i1 := incoming[i-1].Track.ID
		t2 := incoming[i].PlayedAt
		i2 := incoming[i].Track.ID
		if t1.Truncate(30*time.Second).Equal(t2.Truncate(30*time.Second)) && i1 == i2 {
			continue
		}
		outgoing = append(outgoing, incoming[i])
	}

	return outgoing
}

/*averageFloat - as the name suggest it averages vector of floats
 */
func averageFloat(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
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
