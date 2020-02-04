package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	spotify "github.com/chew-z/spotify"
	"github.com/gin-gonic/gin"
)

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
			log.Printf("Failed to get many tracks: %s", err.Error())
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

func cloudRecent(user string) {
	url := os.Getenv("CLOUD_RECENT_FUNCTION")
	token := getJWToken(url)
	cloudRecent := fmt.Sprintf("%s?user=%s", url, user)
	client := &http.Client{}
	req, _ := http.NewRequest("GET", cloudRecent, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
}

/*paginateHistory - is a helper func for paginating
tracks listened to ie. history.
It returns a firestore query for next/previous page
chunk size is set by global variable pageLimit (24)
*/
func paginateHistory(page string, path string, c *gin.Context) *firestore.Query {
	var q firestore.Query // we will be constructing firestore query here
	if page != "0" {
		lastPage, _ := c.Cookie("lastPage") // where we comming from
		if ltString(lastPage, page) {       // check if we go back or forward
			if lastDoc, err := c.Cookie("lastDoc"); err == nil { // lastDoc cookie stores when last track on pages had been playes
				layout := "2006-01-02 15:04:05 -0700 UTC"
				t, err := time.Parse(layout, lastDoc) // convert string from cookie to UTC time
				if err != nil {
					log.Println(err.Error())
				}
				// start query after last track on previous page (if going forward)
				q = firestoreClient.Collection(path).OrderBy("played_at", firestore.Desc).
					StartAfter(t).Limit(pageLimit)
			}
		} else {
			// start query at offset (page size * # tracks on page)
			p, _ := strconv.Atoi(page)
			q = firestoreClient.Collection(path).OrderBy("played_at", firestore.Desc).
				Offset(pageLimit * p).Limit(pageLimit)
		}
	} else {
		// of this is zero page get mist recent tracks (which might have chaged in the meantime
		// so make no assumptions)
		q = firestoreClient.Collection(path).OrderBy("played_at", firestore.Desc).Limit(pageLimit)
	}
	return &q
}

/*navigation - return navigation object to page
- scrolling multi-page results
*/
func getNavigation(page string) *navigation {
	var nav navigation
	if page == "" || page == "0" {
		nav.Previous = ""
		nav.Current = "0"
		nav.Next = "1"
	} else {
		chunk, _ := strconv.Atoi(page)
		nav.Previous = strconv.Itoa(chunk - 1)
		nav.Current = strconv.Itoa(chunk)
		nav.Next = strconv.Itoa(chunk + 1)
	}
	return &nav
}

/*ltString - this liitle function wraps comparision
of two numbers passed as a string (from storing in cookie
and query parameters).
Returns true if first argument is lower then second.
*/
func ltString(a string, b string) bool {
	i, err := strconv.Atoi(a)
	if err != nil {
		i = 0
	}
	j, err := strconv.Atoi(b)
	if err != nil {
		j = 0
	}
	if i < j { // a < b
		return true
	}
	return false
}
