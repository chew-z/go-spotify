package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/thinkerou/favicon"
	"github.com/zmb3/spotify"
)

// const (
// )
var (
	kaszka        = cache.New(15*time.Minute, 1*time.Minute)
	auth          = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate)
	clientChannel = make(chan *spotify.Client)
	redirectURI   = os.Getenv("REDIRECT_URI")
)

func init() {
}

/* Simplified code (no caching or no saving token)
for learning and experimenting with
Spotify (github.com/zmb3/spotify) oauth2 process
using goroutines and channels

TODO - state=/user returns state=abc123
sometimes we are getting wrong state. I have up and think it is due to cookies

*/
func main() {

	router := gin.Default()
	router.Use(favicon.New("./favicon.png"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World! This is auth-gin-cache.go here.")
	})
	router.GET("/whoami", whoami)
	router.GET("/user", user)
	router.GET("/callback", callback)

	router.Run(":8080")
}

/* simple statefull authorization handler using channels
it is expected that parameter state = (FormValue("state")
contains proper endpoint where we should redirect
This is not always the case due to browser/spotify cookies/
imperfect implementattion of oauth2 in go etc.
*/
func callback(c *gin.Context) {
	endpoint := c.Request.FormValue("state")
	log.Printf("/callback: endpoint: %s", endpoint)
	// Now we need different token for each endpoint = state. Sucks big way!
	tok, err := auth.Token(endpoint, c.Request)
	if err != nil {
		c.String(http.StatusForbidden, "Couldn't get token")
		log.Fatal(err)
	}
	/* TODO
	- add getToken i setToken
	tokenToFile(tok)
	- caching Token
	- saving to disk persists betwen restarts
	- add cache client
	*/
	// create copy of gin.Context to be used inside the goroutine
	// cCopy := c.Copy()
	go func() {
		client := auth.NewClient(tok)
		log.Println("/callback: Login Completed!")
		kaszka.Set(endpoint, &client, tok.Expiry.Sub(time.Now()))
		log.Printf("/callback: Cached client for: %s", endpoint)
		clientChannel <- &client
	}()
	url := fmt.Sprintf("http://%s%s?deuce=1", c.Request.Host, endpoint)
	defer c.Redirect(303, url)
	log.Printf("callback: redirecting to endpoint %s", url)
}

/*
 */
func user(c *gin.Context) {
	var client *spotify.Client
	// var err error
	endpoint := c.Request.URL.Path
	client, _ = getClient(endpoint)
	d := c.DefaultQuery("deuce", "0")

	if client == nil {
		// get client from oauth
		if d == "1" {
			// wait for auth to complete
			client = <-clientChannel
		} else {
			url := auth.AuthURL(endpoint)
			c.Redirect(303, url)
			return
		}
	}
	defer func() {
		// use the client to make calls that require authorization
		user, err := client.CurrentUser()
		if err != nil {
			log.Fatal(err)
		}
		msg := fmt.Sprintf("You are logged in as: %s", user.ID)
		log.Println("/user: Login Completed!")
		c.String(http.StatusOK, msg)
	}()
	// log.Println("/auth: Please log in to Spotify by visiting the following page in your browser:", url)
	// HTTP standard does not pass through HTTP headers on an 302/301 directive
	// 303 is never cached and always is GET
}

/*	No redirect - manual copy/paste of authorization link
 */
func whoami(c *gin.Context) {
	endpoint := c.Request.URL.Path
	url := auth.AuthURL(endpoint)
	log.Println("/whoami: Please log in to Spotify by visiting the following page in your browser:", url)
	// wait for auth to complete
	client := <-clientChannel
	{
		// use the client to make calls that require authorization
		user, err := client.CurrentUser()
		if err != nil {
			log.Fatal(err)
		}
		msg := fmt.Sprintf("You are logged in as: %s", user.ID)
		log.Println("/whoami: Login Completed!")
		c.String(http.StatusOK, msg)
	}
}
func getClient(endpoint string) (*spotify.Client, error) {
	if gclient, foundClient := kaszka.Get(endpoint); foundClient {
		log.Printf("Cached client found for: %s", endpoint)
		client := gclient.(*spotify.Client)
		return client, nil
	}
	msg := fmt.Sprintf("No cached client found for: %s", endpoint)
	log.Println(msg)
	return nil, errors.New(msg)
}

// /*	Retrieves a token from a local JSON file.
// 	https://developers.google.com/tasks/quickstart/go
// */
// func tokenFromFile(file string) (*oauth2.Token, error) {
// 	tok := &oauth2.Token{}
// 	f, err := os.Open(file)
// 	defer f.Close()
// 	if err != nil {
// 		return nil, err
// 	}
// 	err = json.NewDecoder(f).Decode(tok)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return tok, nil
// }

// /*
//  */
// func tokenToFile(tok *oauth2.Token) {
// 	jsonToken, jsonErr := json.MarshalIndent(tok, "    ", "    ")
// 	if jsonErr != nil {
// 		log.Println(jsonErr.Error())
// 	}
// 	_ = ioutil.WriteFile("token.json", jsonToken, 0600)
// 	log.Printf("/callback: Token saved to file\n%s", string(jsonToken))
// }
