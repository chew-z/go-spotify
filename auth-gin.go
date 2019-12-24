package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/thinkerou/favicon"
	"github.com/zmb3/spotify"
)

// const (
// 	redirectURI = "http://localhost:8080/callback"
// )

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
	clientChannel := make(chan *spotify.Client)
	redirectURI := os.Getenv("REDIRECT_URI")
	auth := spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate)

	router := gin.Default()
	router.Use(favicon.New("./favicon.png"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello World!")
	})
	router.GET("/abc123", func(c *gin.Context) {
		if d := c.DefaultQuery("deuce", "0"); d == "1" {
			client := <-clientChannel
			{
				user, err := client.CurrentUser()
				if err != nil {
					log.Fatal(err)
				}
				msg := fmt.Sprintf("/abc123: You are logged in as: %s", user.ID)
				c.String(http.StatusOK, msg)
			}
		} else {
			c.String(http.StatusOK, "This page is for handling abc123 glitch")
		}
	})
	/* No redirect - manual copy/paste of authorization link */
	router.GET("/whoami", func(c *gin.Context) {
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
	})
	/* */
	router.GET("/user", func(c *gin.Context) {
		endpoint := c.Request.URL.Path
		url := auth.AuthURL(endpoint)
		if d := c.DefaultQuery("deuce", "0"); d == "1" {
			// wait for auth to complete
			client := <-clientChannel
			{
				// use the client to make calls that require authorization
				user, err := client.CurrentUser()
				if err != nil {
					log.Fatal(err)
				}
				msg := fmt.Sprintf("You are logged in as: %s", user.ID)
				log.Println("/random: Login Completed!")
				c.String(http.StatusOK, msg)
			}
		} else {
			// log.Println("/auth: Please log in to Spotify by visiting the following page in your browser:", url)
			// HTTP standard does not pass through HTTP headers on an 302/301 directive
			// 303 is never cached and always is GET
			c.Redirect(303, url)
		}
	})
	/* simple statefull authorization handler using channels
	it is expected that parameter state = (FormValue("state")
	contains proper endpoint where we should redirect
	This is not always the case due to browser/spotify cookies/
	imperfect implementattion of oauth2 in go etc.
	*/
	router.GET("/callback", func(c *gin.Context) {
		endpoint := c.Request.FormValue("state")
		log.Printf("/callback: endpoint: %s", endpoint)
		tok, err := auth.Token(endpoint, c.Request)
		if err != nil {
			c.String(http.StatusForbidden, "Couldn't get token")
			log.Fatal(err)
		}
		// create copy of gin.Context to be used inside the goroutine
		// cCopy := c.Copy()
		go func() {
			client := auth.NewClient(tok)
			log.Println("/callback: Login Completed!")
			clientChannel <- &client
		}()
		url := fmt.Sprintf("http://%s%s?deuce=1", c.Request.Host, endpoint)
		defer c.Redirect(303, url)
		log.Printf("callback: redirecting to endpoint %s", url)
	})
	router.Run(":8080")
}
