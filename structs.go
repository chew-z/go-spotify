package main

import (
	"time"

	spotify "github.com/chew-z/spotify"
	"golang.org/x/oauth2"
)

type firestoreTrack struct {
	Name     string    `firestore:"track_name"`
	Artists  string    `firestore:"artists"`
	PlayedAt time.Time `firestore:"played_at"`
	ID       string    `firestore:"id,omitempty"`
}

// the name - this is what we need to
// retrieve token form firestore and for some
//initialization
type firestoreToken struct {
	user     string        // Spotify user ID
	country  string        // The country of the user, as set in the user's account profile
	timezone string        // TODO let user set timezone
	path     string        // authorization path (gin routes group)
	token    *oauth2.Token // Spotify token
}

type timeZones struct {
	Time string   `json:"time"`
	Zone []string `json:"zone"`
}

type navigation struct {
	Previous string
	Current  string
	Next     string
}

type recommendationParameters struct {
	Seeds           spotify.Seeds
	TrackAttributes *spotify.TrackAttributes
	FromYear        int
	MinTrackCount   int
}

// TODO - used only once
type popularTrack struct {
	Count int `firestore:"count,omitempty"`
}

// TODO - its just tracks now, not topTracks
type topTrack struct {
	Count   int
	Name    string
	Artists string
	URL     string
	Album   string
	Image   string
}

type audioTrack struct {
	ID               spotify.ID
	Name             string
	Artists          string
	Instrumentalness int
	Acousticness     int
	Energy           int
	Loudness         int
	Tempo            int
	URL              string
	Image            string
}

type userLocation struct {
	Name     string
	URL      string
	Country  string
	Time     string
	UnixTime int64
	Tz       string
	Lat      string
	Lon      string
	City     string
}
