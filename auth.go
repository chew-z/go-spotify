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
	user  string
	path  string
	token *oauth2.Token
}

/*AuthenticationRequired - ..
 */
func AuthenticationRequired(authPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			// c.JSON(http.StatusUnauthorized, gin.H{"error": "user needs to be signed in to access this service"})
			url := auth.AuthURL(authPath)
			log.Println("/auth: Please log in to Spotify by visiting the following page in your browser:", url)
			c.Redirect(303, url)
			c.Abort()
			return
		}
		userString := user.(string)
		log.Printf("/auth: Seems like we are legit as user %s for routes group %s: ", userString, authPath)
		uuid := session.Get("uuid").(string)
		// add session verification here, like checking if the user and authType
		// combination actually exists if necessary. Try adding caching this (redis)
		// since this middleware might be called a lot
		// kaszka.Add(uuid, &client, cache.DefaultExpiration)

		// var client *spotify.Client
		if _, foundClient := kaszka.Get(uuid); foundClient {
			log.Printf("/auth: Cached client found for: %s", uuid)
			// client = gclient.(*spotify.Client)
		} else {
			var newTok firestoreToken
			newTok.user = userString
			newTok.path = authPath
			tok, _ := getTokenFromDB(&newTok)
			log.Printf("/auth: Token expires at: %s", tok.Expiry.In(location).Format("15:04:03"))
			client := auth.NewClient(tok)
			kaszka.Set(uuid, &client, cache.DefaultExpiration)
			if m, _ := time.ParseDuration("5m30s"); time.Until(tok.Expiry) < m {
				newTok.token, _ = client.Token()
				log.Printf("New token expires in: %s", newTok.token.Expiry.Sub(time.Now()).String())
				updateTokenInDB(&newTok)
			}
		}
		c.Next()
	}
}

func clientMagic(c *gin.Context) *spotify.Client {
	endpoint := c.Request.URL.Path
	var client *spotify.Client
	session := sessions.Default(c)
	uuid := session.Get("uuid").(string)
	userString := session.Get("user").(string)
	authPath := session.Get("authPath").(string)
	log.Printf("session id: %s", uuid)

	if gClient, foundClient := kaszka.Get(uuid); foundClient {
		log.Printf("%s: Cached client found for: %s", endpoint, uuid)
		client = gClient.(*spotify.Client)
		kaszka.Add(uuid, client, cache.DefaultExpiration) // this is reset of expiration
		return client
	}
	var newTok firestoreToken
	newTok.user = userString
	newTok.path = authPath
	tok, err := getTokenFromDB(&newTok)
	if err != nil {
		log.Printf("Couldn't find token for %s", newTok.path)
		return nil
	}
	log.Printf("/auth: Token expires at: %s", tok.Expiry.In(location).Format("15:04:03"))
	newClient := auth.NewClient(tok)
	kaszka.Set(uuid, &newClient, cache.DefaultExpiration)
	return &newClient
}

func getTokenFromDB(token *firestoreToken) (*oauth2.Token, error) {
	path := fmt.Sprintf("users/%s/tokens%s", token.user, token.path)
	dsnap, err := firestoreClient.Doc(path).Get(ctx)
	if err != nil {
		log.Printf("Error retrieving token from Firestore for %s %s.\nPossibly it ain't there..", path, err.Error())
		return nil, err
	}
	tok := &oauth2.Token{}
	dsnap.DataTo(tok)
	token.token = tok
	log.Printf("getTokenFromDB: Got token with expiration %s", tok.Expiry.In(location).Format("15:04:05"))
	return tok, nil
}

func saveTokenToDB(token *firestoreToken) {
	path := fmt.Sprintf("users/%s/tokens%s", token.user, token.path)
	_, err := firestoreClient.Doc(path).Set(ctx, token.token)
	_, err = firestoreClient.Collection("users").Doc(token.user).Set(ctx, map[string]interface{}{
		"userID": token.user,
	}, firestore.MergeAll)

	if err != nil {
		log.Printf("saveToken: Error saving token for %s %s", token.path, err.Error())
	} else {
		log.Printf("saveToken: Saved token for %s into Firestore", token.path)
		log.Printf("saveToken: Token expiration %s", token.token.Expiry.In(location).Format("15:04:05"))
	}
}
func updateTokenInDB(token *firestoreToken) {
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

// func getTokenFromDB(path string) (*oauth2.Token, error) {
// 	dsnap, err := firestoreClient.Doc(path).Get(ctx)
// 	if err != nil {
// 		log.Printf("Error retrieving token from Firestore for %s %s.\nPossibly it ain't there..", path, err.Error())
// 		return nil, err
// 	}
// 	tok := &oauth2.Token{}
// 	dsnap.DataTo(tok)
// 	log.Printf("getTokenFromDB: Got token with expiration %s", tok.Expiry.In(location).Format("15:04:05"))
// 	return tok, nil
// }

// func saveTokenToDB(path string, token *oauth2.Token) {
// 	_, err := firestoreClient.Doc(path).Set(ctx, token)

// 	if err != nil {
// 		log.Printf("saveToken: Error saving token for %s %s", path, err.Error())
// 	} else {
// 		log.Printf("saveToken: Saved token for %s into Firestore", path)
// 		log.Printf("saveToken: Token expiration %s", token.Expiry.In(location).Format("15:04:05"))
// 	}
// }

// func updateTokenInDB(path string, token *oauth2.Token) {
// 	_, err := firestoreClient.Doc(path).Set(ctx, map[string]interface{}{
// 		"AccessToken":  token.AccessToken,
// 		"Expiry":       token.Expiry,
// 		"RefreshToken": token.RefreshToken,
// 		"TokenType":    token.TokenType,
// 	}, firestore.MergeAll)

// 	if err != nil {
// 		log.Printf("updateToken: Error saving token for %s %s", path, err.Error())
// 	} else {
// 		log.Printf("updateToken: Saved token for %s into Firestore", path)
// 		log.Printf("updateToken: Token expiration %s", token.Expiry.In(location).Format("15:04:05"))
// 	}
// }
