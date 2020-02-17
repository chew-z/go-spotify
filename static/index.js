// index.js
async function onShare() {
    const { title } = 'Music';
    const url = document.querySelector('link[rel=canonical]')
        ? document.querySelector('link[rel=canonical]').href
        : document.location.href;
    const text = 'Look, I have found companion for Spotify!';

    if (navigator.share) {
        try {
            await navigator.share({
                title,
                url,
                text,
            });
            alert('Thanks for Sharing!');
        } catch (err) {
            /*
          This error will appear if the user cancel the action of sharing.
        */
            alert(`Couldn't share ${err}`);
        }
    } else {
        console.log('Sharing not supported !!');
    }
}

function initializeApp() {
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js').then(() => {
            document.querySelector('#share_button').addEventListener('click', () => {
                onShare();
            });
        });
    }
}

initializeApp();

$(document).ready(() => {
    if (navigator.share) {
        $('#share_button')
            .removeClass('d-none')
            .addClass('d-block');
    } else {
        $('#share_button')
            .removeClass('d-block')
            .addClass('d-none');
    }
});
