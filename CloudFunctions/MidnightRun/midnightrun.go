package midnightrun

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

/*
MidnightRun - does litmus configuration
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
	{
		counter := 0
		iter := firestoreClient.Collection("users").Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			log.Println(err.Error())
			log.Printf("An error retrieving users: %s", err.Error())
		}
		batch := firestoreClient.Batch()
		log.Printf("%d", len(docs))
		for _, doc := range docs {
			user := doc.Data()["userID"].(string)
			p := doc.Data()["premium_user"]
			if p == nil {
				counter++
				log.Println(user)
				docRef := firestoreClient.Collection("users").Doc(user)
				batch.Set(docRef, map[string]interface{}{
					"premium_user": false,
				}, firestore.MergeAll)
			}
		}
		if counter > 0 {
			// Commit the batch.
			_, err = batch.Commit(ctx)
			if err != nil {
				// Handle any errors in an appropriate way, such as returning them.
				log.Printf("An error has occurred: %s", err.Error())
			}
		}
	}
	trackCounter := 0
	userCounter := 0
	{
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
					iterDoc, err := iter.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						log.Println(err.Error())
					}
					batch.Delete(iterDoc.Ref)
					numDeleted++
				}
				trackCounter += numDeleted
				log.Printf("Deleted: %d tracks", numDeleted)
				// If there are no documents to delete, the process is over.
				if numDeleted == 0 {
					break
				}
				// Commit the batch.
				_, err := batch.Commit(ctx)
				if err != nil {
					log.Printf("An error while commiting batch to firestore: %s", err.Error())
				}
			}
			userCounter++
		}
	}
	log.Printf("Processed %d tracks for %d users", trackCounter, userCounter)
	w.WriteHeader(http.StatusOK)
	response := "Midnight Run - starring Robert DeNiro"
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Panic(err)
	}
}

func initFirestoreDatabase(ctx context.Context) *firestore.Client {
	// use Cloud credentials and roles
	firestoreClient, err := firestore.NewClient(ctx, firestore.DetectProjectID)
	if err != nil {
		log.Panic(err)
	}
	return firestoreClient
}
