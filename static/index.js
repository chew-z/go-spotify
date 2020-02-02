// index.js
function onShare() {
    const { title } = 'Music';
    const url = document.querySelector('link[rel=canonical]')
        ? document.querySelector('link[rel=canonical]').href
        : document.location.href;
    const text = 'Look, I have found companion for Spotify!';

    if (navigator.share) {
        navigator
            .share({
                title,
                url,
                text,
            })
            .then(() => {
                alert('Thanks for Sharing!');
            })
            .catch((err) => {
                console.log(`Couldn't share ${err}`);
            });
    } else {
        alert('Not supported !!');
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

$(document).ready(function() {
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
