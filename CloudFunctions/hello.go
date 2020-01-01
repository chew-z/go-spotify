package helloworld

import (
	"net/http"
)

var mux = newMux()

//HelloHTTP represents cloud function entry point
func HelloHTTP(w http.ResponseWriter, r *http.Request) {
	mux.ServeHTTP(w, r)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", zero)
	mux.HandleFunc("/one", one)
	mux.HandleFunc("/two", two)
	mux.HandleFunc("/three", three)

	return mux
}

func zero(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello from /"))
}

func one(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello from one"))
}

func two(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello from two"))
}

func three(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello from three"))
}
