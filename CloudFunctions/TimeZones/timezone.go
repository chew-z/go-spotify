package sunset

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/schema"
	"gopkg.in/ugjka/go-tz.v2/tz"
)

type request struct {
	Lat  float64   `schema:"lat"`
	Lon  float64   `schema:"lon"`
	Date time.Time `schema:"date"`
}

type response struct {
	Time string   `json:"time"`
	Zone []string `json:"zone"`
}

/*TimeZones - a microservice (GCF) that takes
date, lattitude, longtitude and returns timezone(s)
and time at the location.
If date is not set it defaults to current time
while lat/lon defaults to Warsaw/Europe
very fast, everything in memory
returns a JSON-encoded response.
*/
func TimeZones(w http.ResponseWriter, r *http.Request) {
	var decoder = schema.NewDecoder()
	decoder.RegisterConverter(time.Time{}, dateConverter)

	// Parse the request from query string
	var req request
	if err := decoder.Decode(&req, r.URL.Query()); err != nil {
		// Report any parsing errors
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintf(w, "Error: %s", err)
		return
	}
	if req.Date.IsZero() {
		req.Date = time.Now()
	}
	if req.Lat == 0 && req.Lon == 0 {
		req.Lat = 52.237
		req.Lon = 21.017
	}
	// log.Printf("Date %v, Lat %v, Lon %v", req.Date, req.Lat, req.Lon)
	zone, err := tz.GetZone(tz.Point{
		Lon: req.Lon, Lat: req.Lat,
	})
	if err != nil {
		log.Panic(err)
	}
	location, _ := time.LoadLocation(zone[0])
	time := req.Date.In(location).Format("15:04:05")
	// Send response back as JSON
	w.WriteHeader(http.StatusOK)
	response := response{time, zone}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		panic(err)
	}
}

func dateConverter(value string) reflect.Value {
	s, _ := time.Parse("2006-01-_2", value)
	return reflect.ValueOf(s)
}
