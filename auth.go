package main

import (
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

type firestoreToken struct {
	user     string        // Spotify user ID
	country  string        // The country of the user, as set in the user's account profile
	timezone string        // TODO let user set timezone
	path     string        // authorization path (gin routes group)
	token    *oauth2.Token // Spotify token
}

/*TODO
The big unknown is what happens when user deauthorizes our app in preferences
without letting us know
Token becomes invalid and throws at us errors at each attempt (this is a problem for
Cloud functions which will still attempt to authorize
- We should also get user country and location
*/

/*AuthenticationRequired - is an authentication middleware for selected paths
authPath ("/user" by default) is used for gin router group authorized
*/
func AuthenticationRequired(authPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")
		// if user is nil == user is not logged in with Spotify, so start authorization process
		if user == nil {
			// c.JSON(http.StatusUnauthorized, gin.H{"error": "user needs to be signed in to access this service"})
			url := auth.AuthURL(authPath)
			log.Println("/auth: Please log in to Spotify by visiting the following page in your browser:", url)
			c.Redirect(303, url)
			c.Abort()
			// we call c.Abort() if the user is unauthenticated/unauthorized.
			// This is because gin calls the next function in the chain even after you write the header
			return
		}
		// add session verification here, like checking if the user and authPath
		// combination actually exists if necessary. Try adding caching this
		// since this middleware might be called a lot

		// if user is set in session authPath and uuid should be (casting to string will fail terribly if nil)
		userString := user.(string)
		if path := session.Get("authPath").(string); path == authPath {
			log.Printf("/auth: Seems like we are legit as user %s for routes group %s: ", userString, authPath)
		}
		uuid := session.Get("uuid").(string)
		// var client *spotify.Client
		// We are only checking if there is a client for this session in cache
		if _, foundClient := kaszka.Get(uuid); foundClient {
			// client = gclient.(*spotify.Client)
			// kaszka.Add(uuid, &client, cache.DefaultExpiration) //Add is like Set or refresh
		} else { // and if there is no client in cache we get token from Firestore
			log.Printf("/auth: Cached client NOT found for: %s", uuid)
			// create client and put it in cache
			var newTok firestoreToken
			newTok.user = userString
			newTok.path = authPath
			tok, _ := getTokenFromDB(&newTok)
			log.Printf("/auth: Token expires at: %s", tok.Expiry.In(location).Format("15:04:03"))
			spotifyClient := auth.NewClient(tok)
			kaszka.Set(uuid, &spotifyClient, cache.DefaultExpiration)
			// if token in Firestore is close to or past expiration we refresh token and update in database
			if m, _ := time.ParseDuration("4m30s"); time.Until(tok.Expiry) < m {
				newTok.token, _ = spotifyClient.Token()
				log.Printf("New token expires in: %s", newTok.token.Expiry.Sub(time.Now()).String())
				updateTokenInDB(&newTok)
			}
		}
		return
		// c.Next() //TODO - philosophical question - Is c.Next() needed here?
		// https://github.com/gin-gonic/gin/issues/1169
	}
}

/*clientMagic - is how endpoints obtain Spotify client
which is from cache (fast and cheap in resources) or by
retrieving token from Firestore and creating new client (slow)
*/
func clientMagic(c *gin.Context) *spotify.Client {
	endpoint := c.Request.URL.Path
	var client *spotify.Client
	session := sessions.Default(c)
	// we are past authorization middleware so session
	// variables should not be nil and we can safely cast
	// them as strings
	uuid := session.Get("uuid").(string)
	userString := session.Get("user").(string)
	authPath := session.Get("authPath").(string)
	log.Printf("/clientMagic: session id: %s", uuid)
	// If the session is running Spotify client is probably cached
	if gClient, foundClient := kaszka.Get(uuid); foundClient {
		client = gClient.(*spotify.Client)
		kaszka.SetDefault(uuid, client) // replace existing
		return client
	}
	log.Printf("%s: Cached client NOT found for: %s", endpoint, uuid)
	// if client isn't in cache get token from database (user should be logged in and Spotify token saved)
	var newTok firestoreToken
	newTok.user = userString
	newTok.path = authPath
	tok, err := getTokenFromDB(&newTok)
	if err != nil { // This is irregular will lead to I am teapot.
		log.Printf("Couldn't find token for %s", newTok.path)
		return nil
	}
	log.Printf("/clientMagic: Token in Firestore expires at: %s", tok.Expiry.In(location).Format("15:04:03"))
	newClient := auth.NewClient(tok)
	// if an item doesn't already exist for the given key, or if the existing item has expired
	kaszka.Add(uuid, &newClient, cache.DefaultExpiration)
	return &newClient
}

/*getTokenFromDB - retrieves token fromFirestore
 */
func getTokenFromDB(token *firestoreToken) (*oauth2.Token, error) {
	// we need Spotify user ID and router group path (auth group) to retrieve a token
	path := fmt.Sprintf("users/%s/tokens%s", token.user, token.path)
	dsnap, err := firestoreClient.Doc(path).Get(ctx)
	if err != nil {
		log.Printf("Error retrieving token from Firestore for %s %s.\nPossibly it ain't there..", path, err.Error())
		return nil, err
	}
	tok := &oauth2.Token{}
	dsnap.DataTo(tok)
	token.token = tok // here token is set by reference and also returned in input parameter
	log.Printf("getTokenFromDB: Got token with expiration %s", tok.Expiry.In(location).Format("15:04:05"))
	return tok, nil
}

/*saveTokenToDb - saves token to Firestore
 */
func saveTokenToDB(token *firestoreToken) {
	// we need Spotify user ID and router group path (auth group) to retrieve a token
	path := fmt.Sprintf("users/%s/tokens%s", token.user, token.path)
	// TODO - two set operations - ?
	_, err := firestoreClient.Doc(path).Set(ctx, token.token)
	_, err = firestoreClient.Collection("users").Doc(token.user).Set(ctx, map[string]interface{}{
		"userID":   token.user,
		"country":  token.country,
		"timezone": token.timezone,
	}, firestore.MergeAll)
	if err != nil {
		log.Printf("saveToken: Error saving token for %s %s", token.path, err.Error())
	} else {
		log.Printf("saveToken: Saved token for %s into Firestore", token.path)
		log.Printf("saveToken: Token expiration %s", token.token.Expiry.In(location).Format("15:04:05"))
	}
}

/*updateTokenInDB - updates token in Firestore
 */
func updateTokenInDB(token *firestoreToken) {
	// we need Spotify user ID and router group path (auth group) to retrieve a token
	path := fmt.Sprintf("users/%s/tokens%s", token.user, token.path)
	_, err := firestoreClient.Doc(path).Set(ctx, map[string]interface{}{
		"AccessToken":  token.token.AccessToken,
		"Expiry":       token.token.Expiry,
		"RefreshToken": token.token.RefreshToken,
		"TokenType":    token.token.TokenType,
	}, firestore.MergeAll)

	if err != nil {
		log.Printf("updateToken: Error saving token for %s %s", path, err.Error())
	} else {
		log.Printf("updateToken: Saved token for %s into Firestore", path)
		log.Printf("updateToken: Token expiration %s", token.token.Expiry.In(location).Format("15:04:05"))
	}
}
