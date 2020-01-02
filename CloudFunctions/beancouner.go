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
	"cloud.google.com/go/functions/metadata"
	firebase "firebase.google.com/go"
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

// GCLOUD_PROJECT is automatically set by the Cloud Functions runtime.
var projectID = os.Getenv("GCLOUD_PROJECT")

// client is a Firestore client, reused between function invocations.
var firestoreClient *firestore.Client

//TODO - it works as advertised! it is also useless and dangerous as it creates infinite loop

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

// HelloFirestore is triggered by a change to a Firestore document.
func HelloFirestore(ctx context.Context, e FirestoreEvent) error {
	meta, err := metadata.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("metadata.FromContext: %v", err)
	}

	log.Printf("Function triggered by change to: %v", meta.Resource)
	log.Printf("Old value: %+v", e.OldValue)
	log.Printf("New value: %+v", e.Value)

	fullPath := strings.Split(e.Value.Name, "/documents/")[1]
	log.Println(fullPath)
	pathParts := strings.Split(fullPath, "/")
	// collection := pathParts[0]
	doc := strings.Join(pathParts[1:], "/") // aka track.ID
	log.Println(doc)
	// In order to avoid triggering infinite loop we keep counters in separate collection
	docRef := firestoreClient.Collection("popular_tracks").Doc(doc)
	w, err := docRef.Set(ctx, map[string]interface{}{
		"count": firestore.Increment(1)}, firestore.MergeAll)
	// w, err := docRef.Update(ctx, []firestore.Update{
	// 	{Path: "count", Value: firestore.Increment(1)},
	// 	// TODO - !!!
	// 	// https://cloud.google.com/functions/docs/calling/cloud-firestore#specifying_the_document_path
	// 	// Functions only respond to document changes, and cannot monitor specific fields or collections.
	// 	// So this creates infinite loop updating "count" forever
	// })
	if err != nil {
		log.Println(err.Error())
		return fmt.Errorf("beancounter: %v", err)
	}
	log.Println(w.UpdateTime)
	return nil
}
