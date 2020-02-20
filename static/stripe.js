let stripe;
let checkoutSessionId;

async function setupElements() {
    const response = await fetch('/stripe-public-key', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
    });

    return await response.json();
}

async function createCheckoutSession(isDonation) {
    const response = await fetch('/create-checkout-session', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ isDonation }),
    });

    return await response.json();
}

setupElements().then((data) => {
    stripe = Stripe(data.publicKey);
});

createCheckoutSession(false).then((data) => {
    checkoutSessionId = data.checkoutSessionId;
});

$('input[name=donation]').change(() => {
    if ($(this).is(':checked')) {
        // Checkbox is checked..
        createCheckoutSession(true);
        $('#order-total').text('€22.90');
    } else {
        // Checkbox is not checked..
        createCheckoutSession(false);
        $('#order-total').text('€12.90');
    }
});

// const donation = document.querySelector('input[name=donation]');
// document.querySelector('input[name="subscribe"]').addEventListener('change', (evt) => {
//     if (this.checked) {
//         createCheckoutSession(true);
//         document.getElementById('order-total').textContent = '€22.90'; // Because they are buying the extra item
//     } else {
//         createCheckoutSession(false);
//         document.getElementById('order-total').textContent = '€12.90'; // Not buying the extra item
//     }
// });

document.querySelector('#submit').addEventListener('click', (evt) => {
    evt.preventDefault();
    // Initiate payment
    stripe
        .redirectToCheckout({
            sessionId: checkoutSessionId,
        })
        .then((result) => {
            console.log('error');
            // If `redirectToCheckout` fails due to a browser or network
            // error, display the localized error message to your customer
            // using `result.error.message`.
        })
        .catch((err) => {
            console.log(err);
        });
});
