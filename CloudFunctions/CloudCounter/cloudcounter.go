// Package beancounter contains a Cloud Function triggered by a Firestore event.
package beancounter

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

// FirestoreEvent is the payload of a Firestore event.
type FirestoreEvent struct {
	OldValue   FirestoreValue `json:"oldValue"`
	Value      FirestoreValue `json:"value"`
	UpdateMask struct {
		FieldPaths []string `json:"fieldPaths"`
	} `json:"updateMask"`
}

// FirestoreValue holds Firestore fields.
type FirestoreValue struct {
	CreateTime time.Time `json:"createTime"`
	// Fields is the data for this value. The type depends on the format of your
	// database. Log the interface{} value and inspect the result to see a JSON
	// representation of your database fields.
	Fields     interface{} `json:"fields"`
	Name       string      `json:"name"`
	UpdateTime time.Time   `json:"updateTime"`
}

// Fields:map[
// track_name:map[stringValue:Skipping]
// artists:map[stringValue:Yosi Horikawa]
// counter:map[integerValue:15828]
// played_at:map[timestampValue:2020-01-01T21:26:18.974Z]
// ]

// type firestoreTrack struct {
// 	Name     string    `firestore:"track_name"`
// 	Artists  string    `firestore:"artists"`
// 	PlayedAt time.Time `firestore:"played_at"`
// 	Count    int       `firestore:"count,omitempty"`
// }

var (
	firestoreClient *firestore.Client
	ctx             = context.Background()
)

func main() {
	defer firestoreClient.Close()
}

func init() {
	ctx := context.Background()
	firestoreClient = initFirestoreDatabase(ctx)
}

// CloudCounter is triggered by a change to a Firestore document.
func CloudCounter(ctx context.Context, e FirestoreEvent) error {
	fullPath := strings.Split(e.Value.Name, "/documents/")[1]
	pathParts := strings.Split(fullPath, "/")
	userID := pathParts[1]
	docID := pathParts[3]
	// log.Printf("userID and docID: %s %s", userID, docID)
	// In order to avoid triggering infinite loop we keep counters in separate collection
	path := fmt.Sprintf("users/%s/popular_tracks", userID)
	// log.Println(path)
	docRef := firestoreClient.Collection(path).Doc(docID)
	_, err := docRef.Set(ctx, map[string]interface{}{
		"count": firestore.Increment(1)}, firestore.MergeAll)
	// https://cloud.google.com/functions/docs/calling/cloud-firestore#specifying_the_document_path
	// Functions only respond to document changes, and cannot monitor specific fields or collections.
	if err != nil {
		log.Println(err.Error())
		return fmt.Errorf("CloudCounter: %v", err)
	}
	// log.Println(w.UpdateTime)
	return nil
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
