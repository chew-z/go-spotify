package main

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"

	"github.com/zmb3/spotify"
)

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

func appendIfUnique(slice []spotify.ID, i spotify.ID) []spotify.ID {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

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
