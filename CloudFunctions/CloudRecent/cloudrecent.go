package cloudrecent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

type firestoreToken struct {
	user  string
	path  string
	token *oauth2.Token
}

var (
	ctx             = context.Background()
	firestoreClient *firestore.Client
	redirectURI     = os.Getenv("REDIRECT_URI")
	auth            = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed, spotify.ScopePlaylistModifyPublic, spotify.ScopePlaylistModifyPrivate)
)

func main() {
	defer firestoreClient.Close()
}

func init() {
	ctx := context.Background()
	firestoreClient = initFirestoreDatabase(ctx)
}

/*CloudRecent - ..
 */
func CloudRecent(w http.ResponseWriter, r *http.Request) {
	var client spotify.Client
	var newTok firestoreToken
	users, ok := r.URL.Query()["user"]
	if !ok || len(users[0]) < 1 {
		// Process all users
		iter := firestoreClient.Collection("users").Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			log.Println(err.Error())
		}
		trackCounter := 0
		userCounter := 0
		for _, doc := range docs {
			user := doc.Data()["userID"].(string)
			log.Printf("user: %s", user)
			newTok.user = user
			newTok.path = "/user"
			tok, err := getTokenFromDB(&newTok)
			if err != nil {
				log.Println(err.Error())
			}
			client = auth.NewClient(tok)
			recentlyPlayed, err := client.PlayerRecentlyPlayed()
			if err != nil {
				log.Panic(err)
			}
			// Get a new write batch.
			path := fmt.Sprintf("users/%s/recently_played", user)
			batch := firestoreClient.Batch()
			for _, item := range recentlyPlayed {
				artists := joinArtists(item.Track.Artists, ", ")
				playedAt := item.PlayedAt
				recentlyPlayedRef := firestoreClient.Collection(path).Doc(string(item.Track.ID))
				batch.Set(recentlyPlayedRef, map[string]interface{}{
					"played_at":  playedAt,
					"track_name": item.Track.Name,
					"artists":    artists,
					"id":         string(item.Track.ID),
				}, firestore.MergeAll) // Overwrite only the fields in the map; preserve all others.
				trackCounter++
			}
			userCounter++
			// Commit the batch.
			_, errBatch := batch.Commit(ctx)
			if errBatch != nil {
				// Handle any errors in an appropriate way, such as returning them.
				log.Printf("An error while commiting batch to firestore: %s", err.Error())
				log.Panic(err)
			}
		}
		log.Printf("Processed %d tracks for %d users", trackCounter, userCounter)
	} else { // for single user
		user := users[0]
		log.Printf("user: %s", user)
		newTok.user = user
		newTok.path = "/user"
		tok, err := getTokenFromDB(&newTok)
		if err != nil {
			log.Println(err.Error())
		}
		client = auth.NewClient(tok)
		recentlyPlayed, err := client.PlayerRecentlyPlayed()
		if err != nil {
			log.Println(err.Error())
		}
		trackCounter := 0
		path := fmt.Sprintf("users/%s/recently_played", user)
		batch := firestoreClient.Batch()
		for _, item := range recentlyPlayed {
			artists := joinArtists(item.Track.Artists, ", ")
			playedAt := item.PlayedAt
			recentlyPlayedRef := firestoreClient.Collection(path).Doc(string(item.Track.ID))
			batch.Set(recentlyPlayedRef, map[string]interface{}{
				"played_at":  playedAt,
				"track_name": item.Track.Name,
				"artists":    artists,
				"id":         string(item.Track.ID),
			}, firestore.MergeAll) // Overwrite only the fields in the map; preserve all others.
			trackCounter++
		}
		// Commit the batch.
		_, errBatch := batch.Commit(ctx)
		if errBatch != nil {
			// Handle any errors in an appropriate way, such as returning them.
			log.Printf("An error while commiting batch to firestore: %s", err.Error())
			log.Panic(err)
		}
	}
	w.WriteHeader(http.StatusOK)
	response := "OK"
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Println(err.Error())
	}
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
	// log.Printf("getTokenFromDB: Got token with expiration %s", tok.Expiry.In(location).Format("15:04:05"))
	return tok, nil
}

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

/*initFirestoreDatabase - as the name says creates Firestore client
in Google Cloud it is using project ID, on localhost credentials file
*/
func initFirestoreDatabase(ctx context.Context) *firestore.Client {
	// sa := option.WithCredentialsFile(".firebase-credentials.json")
	firestoreClient, err := firestore.NewClient(ctx, os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if err != nil {
		log.Panic(err)
	}
	return firestoreClient
}
