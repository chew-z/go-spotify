package cloudrecent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

var (
	// GCLOUD_PROJECT is automatically set by the Cloud Functions runtime.
	projectID       = os.Getenv("GOOGLE_CLOUD_PROJECT")
	ctx             = context.Background()
	firestoreClient *firestore.Client
	location, _     = time.LoadLocation("Europe/Warsaw")
	redirectURI     = os.Getenv("REDIRECT_URI")
	auth            = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed, spotify.ScopePlaylistModifyPublic, spotify.ScopePlaylistModifyPrivate)
)

type firestoreToken struct {
	user  string
	path  string
	token *oauth2.Token
}

func init() {
	// Use the application default credentials.
	conf := &firebase.Config{ProjectID: projectID}
	// Use context.Background() because the app/client should persist across invocations.
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Fatalf("firebase.NewApp: %v", err)
	}

	firestoreClient, err = app.Firestore(ctx)
	if err != nil {
		log.Fatalf("app.Firestore: %v", err)
	}
}

/*CloudRecent - ..
 */
func CloudRecent(w http.ResponseWriter, r *http.Request) {
	iter := firestoreClient.Collection("users").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		log.Println(err.Error())
	}
	var newTok firestoreToken
	trackCounter := 0
	userCounter := 0
	for _, doc := range docs {
		var client spotify.Client
		// fmt.Println(i, doc.Ref.ID)
		user := doc.Data()["userID"].(string)
		log.Printf("user: %s", user)
		newTok.user = user
		newTok.path = "/user"
		tok, err := getTokenFromDB(&newTok)
		if err != nil {
			log.Println(err.Error())
		}
		// log.Printf("%v", tok)
		client = auth.NewClient(tok)
		// spotifyUser, err := client.CurrentUser()
		// fmt.Println(spotifyUser.DisplayName)
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
			}, firestore.MergeAll) // Overwrite only the fields in the map; preserve all others.
			// log.Printf("%s", item.Track.Name)
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
		log.Printf("Processed %d tracks for %d users", trackCounter, userCounter)
	}
	w.WriteHeader(http.StatusOK)
	response := "OK"
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Panic(err)
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
