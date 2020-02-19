package stripecheckout

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/stripe/stripe-go/webhook"
)

var (
	firestoreClient *firestore.Client
	projectID       = os.Getenv("GOOGLE_CLOUD_PROJECT")
)

func main() {
}
func init() {
	// Use the application default credentials.
	conf := &firebase.Config{ProjectID: projectID}
	// Use context.Background() because the app/client should persist across
	// invocations.
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Panicf("firebase.NewApp: %v", err)
	}
	firestoreClient, err = app.Firestore(ctx)
	if err != nil {
		log.Fatalf("app.Firestore: %v", err)
	}
}

/*StripeCheckout - ...
 */
func StripeCheckout(w http.ResponseWriter, r *http.Request) {
	defer firestoreClient.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(b, r.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if err != nil {
		log.Printf("webhook.ConstructEvent: %s", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if event.Type != "checkout.session.completed" {
		response := fmt.Sprintf("Wrong event type. Expected: checkout.session.completed")
		log.Println(response)
		http.Error(w, response, http.StatusBadRequest)
		return
	}

	user := event.GetObjectValue("client_reference_id")
	if user != "" {
		ctx := context.Background()
		_, err = firestoreClient.Collection("users").Doc(user).Set(ctx, map[string]interface{}{
			"premium_user":         true,
			"subscription_expires": time.Now().AddDate(1, 0, 15), // a year plus two weeks plus a day
		}, firestore.MergeAll)
	} else {
		log.Println("user ID is missing!")
	}
	response := fmt.Sprintf("Checkout completed for %s user", user)
	log.Println(response)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		log.Panic(err)
	}
}
