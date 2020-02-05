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
	"google.golang.org/api/iterator"
)

var (
	firestoreClient *firestore.Client
	ctx             = context.Background()
)

func main() {
}

func init() {
	ctx := context.Background()
	firestoreClient = initFirestoreDatabase(ctx)
}

/*MidnightRun - does litmus configuration
also takes care of database maintenance
It removes all tracks listened to more then a week ago
TODO - it should also clean unused and failed tokens
and users who stoped using service
As the name suggest it is supposed to run once a day at
midnight. But no hassle to run it by hand.

PS. The movie Midnight Run is great https://youtu.be/LF8cT6ivlr4
*/
func MidnightRun(w http.ResponseWriter, r *http.Request) {
	defer firestoreClient.Close()
	trackCounter := 0
	userCounter := 0
	iter := firestoreClient.Collection("users").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		log.Println(err.Error())
		log.Printf("An error retrieving users: %s", err.Error())
	}
	for _, doc := range docs {
		user := doc.Data()["userID"].(string) // legit would to read data and get userID
		log.Printf("user: %s", user)
		batchSize := 50
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
			log.Printf("Deleted: %d", numDeleted)
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
		userCounter++
	}
	log.Printf("Processed %d tracks for %d users", trackCounter, userCounter)
	w.WriteHeader(http.StatusOK)
	response := "Midnight Run - starring Robert DeNiro"
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Panic(err)
	}
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
