package midnightrun

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
)

type firestoreToken struct {
	user     string        // Spotify user ID
	country  string        // The country of the user, as set in the user's account profile
	timezone string        // TODO let user set timezone
	path     string        // authorization path (gin routes group)
	token    *oauth2.Token // Spotify token
}

type firestoreUser struct {
	id       string `firestore:"userID"`
	timezone string `firestore:"timezone,omitempty"`
	country  string `firestore:"country,omitempty"`
}

var (
	projectID       = os.Getenv("GOOGLE_CLOUD_PROJECT")
	firestoreClient *firestore.Client
	ctx             = context.Background()
	location, _     = time.LoadLocation("Europe/Warsaw")
	redirectURI     = os.Getenv("REDIRECT_URI")
	auth            = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate, spotify.ScopeUserTopRead, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead, spotify.ScopeUserReadRecentlyPlayed, spotify.ScopePlaylistModifyPublic, spotify.ScopePlaylistModifyPrivate)
)

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

/*MidnightRun - does litmus configuration
also taks care of database maintenance
It removes all tracks listened to more then a week ago
TODO - it should also clean unused and failed tokens
and users who stoped using service
As the name suggest it is supposed to run once a day at
midnight. But no hassle to run it by hand.

PS. The movie is great https://youtu.be/LF8cT6ivlr4
*/
func MidnightRun(w http.ResponseWriter, r *http.Request) {
	var newTok firestoreToken
	trackCounter := 0
	userCounter := 0
	iter := firestoreClient.Collection("users").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		log.Println(err.Error())
		log.Printf("An error retrieving users: %s", err.Error())
	}
	for _, doc := range docs {
		// var spotifyClient spotify.Client
		// fmt.Println(i, doc.Ref.ID)
		user := doc.Data()["userID"].(string) // legit would to read data and get userID
		log.Printf("user: %s", user)
		newTok.user = user
		newTok.path = "/user"
		// tok, err := getTokenFromDB(&newTok)
		if err != nil {
			log.Println(err.Error())
		}
		// // log.Printf("%v", tok)
		// spotifyClient = auth.NewClient(tok)
		// spotifyUser, err := client.CurrentUser()
		// fmt.Println(spotifyUser.DisplayName)
		// -----------------------------
		batchSize := 20
		// user, err := spotifyClient.CurrentUser()
		if err != nil {
			log.Panic(err)
		}
		path := fmt.Sprintf("users/%s/recently_played", user)
		ref := firestoreClient.Collection(path).Where("played_at", "<", time.Now().AddDate(0, 0, -7)) // 7 days
		for {
			// Get a batch of documents
			iter := ref.Limit(batchSize).Documents(ctx)
			numDeleted := 0
			// Iterate through the documents, adding a delete operation
			// for each one to a WriteBatch.
			batch := firestoreClient.Batch()
			for {
				doc, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Println(err.Error())
				}
				batch.Delete(doc.Ref)
				numDeleted++
			}
			trackCounter += numDeleted
			// If there are no documents to delete, the process is over.
			if numDeleted == 0 {
				break
			}
			// Commit the batch.
			_, err := batch.Commit(ctx)
			if err != nil {
				log.Printf("An error while commiting batch to firestore: %s", err.Error())
				log.Panic(err)
			}
		}
		//--------------------------------
		userCounter++
		// _, errBatch := batch.Commit(ctx)
		// if errBatch != nil {
		// 	// Handle any errors in an appropriate way, such as returning them.
		// 	log.Printf("An error while commiting batch to firestore: %s", err.Error())
		// 	log.Panic(err)
		// }
		log.Printf("Processed %d tracks for %d users", trackCounter, userCounter)
	}
	w.WriteHeader(http.StatusOK)
	response := "Midnight Run - starring Robert DeNiro"
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Panic(err)
	}
}

/*initFirestoreDatabase - as the name says creates Firestore client
in Google Cloud it is using project ID, on localhost credentials file
*/
// func initFirestoreDatabase(ctx context.Context) *firestore.Client {
// 	sa := option.WithCredentialsFile(".firebase-credentials.json")
// 	firestoreClient, err := firestore.NewClient(ctx, os.Getenv("GOOGLE_CLOUD_PROJECT"), sa)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	return firestoreClient
// }

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
