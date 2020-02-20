package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go"
	stripeSession "github.com/stripe/stripe-go/checkout/session"
)

var ()

func handleCreateCheckoutSession(c *gin.Context) {
	endpoint := c.Request.URL.Path
	var req struct {
		IsBuyingSticker bool `json:"isBuyingSticker"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		log.Printf("json.NewDecoder.Decode: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	spotifyClient := clientMagic(c)
	if spotifyClient != nil {
		u, err := spotifyClient.CurrentUser()
		if err != nil {
			log.Panic(err)
		}
		userID := string(u.ID)
		userEmail := u.Email
		// userName := u.DisplayName
		// log.Printf("User: %s, email: %s", userID, userEmail)
		params := &stripe.CheckoutSessionParams{
			PaymentMethodTypes: stripe.StringSlice([]string{
				"card",
			}),
			SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
				Items: []*stripe.CheckoutSessionSubscriptionDataItemsParams{
					&stripe.CheckoutSessionSubscriptionDataItemsParams{
						Plan: stripe.String(os.Getenv("SUBSCRIPTION_PLAN_ID")),
					},
				},
			},
			// TODO - paymentsuccess is for testing, redirect to /user in production
			// SuccessURL:        stripe.String("https://" + os.Getenv("CUSTOM_DOMAIN") + "/paymentsuccess?session_id={CHECKOUT_SESSION_ID}"),
			SuccessURL:        stripe.String("https://" + os.Getenv("CUSTOM_DOMAIN") + "/user"),
			CancelURL:         stripe.String("https://" + os.Getenv("CUSTOM_DOMAIN") + "/user"),
			ClientReferenceID: stripe.String(userID),
			CustomerEmail:     stripe.String(userEmail), //TODO - this is unverified email. Is is necessary? CustomerEmail or Customer not both
		}
		if req.IsBuyingSticker {
			params.LineItems = []*stripe.CheckoutSessionLineItemParams{
				&stripe.CheckoutSessionLineItemParams{
					Name:        stripe.String("Donation"),
					Description: stripe.String("Discretionary donation to suka.yoga"),
					Quantity:    stripe.Int64(1),
					Amount:      stripe.Int64(1000),
					Currency:    stripe.String(string(stripe.CurrencyEUR)),
				},
			}
		}

		stripeSess, err := stripeSession.New(params)
		if err != nil {
			log.Printf("session.New: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"checkoutSessionId": stripeSess.ID})
		return
	}
	c.JSON(http.StatusTeapot, gin.H{endpoint: "failed to find  client"})
}

func handlePublicKey(c *gin.Context) {
	publicKey := os.Getenv("STRIPE_PUBLISHABLE_KEY")
	c.JSON(http.StatusOK, gin.H{"publicKey": publicKey})
}

func handleCheckoutSession(c *gin.Context) {
	id := c.Query("sessionId")
	if id == "" {
		log.Printf("CheckoutSession ID is missing from URL %s", c.Request.RequestURI)
		c.JSON(http.StatusBadRequest, gin.H{"error": http.StatusText(http.StatusBadRequest)})
		return
	}
	// Fetch the CheckoutSession object from your success page
	// to get details about the order
	stripeSess, err := stripeSession.Get(id, nil)
	if err != nil {
		log.Printf("An error happened when getting the CheckoutSession %q from Stripe: %v", id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": http.StatusText(http.StatusBadRequest)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"CheckoutSession": stripeSess})
}

// func handleWebhook(c *gin.Context) {
// 	b, err := ioutil.ReadAll(c.Request.Body)
// 	if err != nil {
// 		log.Printf("ioutil.ReadAll: %v", err)
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}

// 	event, err := webhook.ConstructEvent(b, c.Request.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
// 	if err != nil {
// 		log.Printf("webhook.ConstructEvent: %s", err.Error())
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}

// 	if event.Type != "checkout.session.completed" {
// 		return
// 	}

// 	cust, err := customer.Get(event.GetObjectValue("customer"), nil)
// 	if err != nil {
// 		log.Printf("customer.Get: %v", err)
// 		return
// 	}

// 	if event.GetObjectValue("display_items", "0", "custom") != "" &&
// 		event.GetObjectValue("display_items", "0", "custom", "name") == "Donation" {
// 		log.Printf("🔔 Customer is subscribed and made a donation! Send the thank you note to %s", cust.Email)
// 	} else {
// 		log.Printf("🔔 Customer is subscribed but did not made a donation.")
// 	}
// }
