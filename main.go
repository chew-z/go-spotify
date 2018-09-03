package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "strings"
    "time"
    "golang.org/x/oauth2"
    "github.com/gin-gonic/gin"
    "github.com/patrickmn/go-cache"
    "github.com/zmb3/spotify"
)

// redirectURI is the OAuth redirect URI for the application.
// You must register an application at Spotify's developer portal
// and enter this value.
const redirectURI = "http://localhost:8080/callback"

var (
    // Create a cache with a default expiration time of 15 minutes
    kacha = cache.New(15*time.Minute, 30*time.Minute)
    auth  = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadPrivate)
    client spotify.Client
    state = "abc123"
    endpoint = "/user"
)

func main() {}

func init() {
    // first start an HTTP server
    r := gin.New()

    r.GET("/", func(c *gin.Context) {
        c.String(http.StatusOK, "Hello World!")
    })
    r.GET("/ping", func(c *gin.Context) {
        c.String(http.StatusOK, "pong")
    })
    r.GET("/user", func(c *gin.Context) {
        tok, tokenErr := getToken()
        if tokenErr != nil {
            log.Println(tokenErr.Error())
            c.String(http.StatusOK, fmt.Sprintln("Man, you don't have token"))
        }
        client = auth.NewClient(tok)
        user, err := client.CurrentUser()
        if err != nil {
            log.Println(err.Error())
            c.String(http.StatusOK, fmt.Sprintln("Man, you are no one"))
        } else {
            c.String(http.StatusOK, fmt.Sprintln("Man, you are", user.ID))
        }
    })
    r.GET("/auth", func(c *gin.Context) {
        url := auth.AuthURL(state)
        log.Println("Please log in to Spotify by visiting the following page in your browser:", url)
        c.Redirect(http.StatusMovedPermanently, url)
    })
    r.GET("/callback", func(c *gin.Context) {
        tok, err := auth.Token(state, c.Request)
        if err != nil {
            c.String(http.StatusForbidden, "Couldn't get token")
            log.Fatal(err.Error())
        }
        if st := c.Query("state"); st != state {
            c.String(http.StatusNotFound, "State mismatch")
            log.Fatalf("State mismatch: %s != %s\n", st, state)
        }
        client = auth.NewClient(tok)
        jsonToken, jsonErr := json.MarshalIndent(tok, "    ", "    ")
        if jsonErr != nil {
            log.Println(jsonErr.Error())
        }
        log.Println(string(jsonToken))
        _ = ioutil.WriteFile("token.json", jsonToken, 0644)
        kacha.Set("token", tok, cache.DefaultExpiration)
        // c.SetCookie("token", tok.AccessToken, 3600, endpoint, "localhost", false, true) 
        // c.String(http.StatusOK, "Login Completed!")
        url := fmt.Sprintf("http://%s%s", c.Request.Host, endpoint)
        log.Println(url)
        // both approaches work but 2nd isn't rewriting url
        // defer redirect until finish otherwise we are looping ...
        defer c.Redirect(http.StatusMovedPermanently, url)
        // c.Request.URL.Path = endpoint
        // r.HandleContext(c)
    })
    r.GET("/top", func(c *gin.Context) {
        tok, tokenErr := getToken()
        if tokenErr != nil {
            log.Println("Not found token. Authenticating first")
            endpoint = "/top"
            c.Request.URL.Path = "/auth"
            r.HandleContext(c)
            return
        }
        client = auth.NewClient(tok)
        top, err := client.CurrentUsersPlaylists()
        if err != nil {
            log.Println(err.Error())
            c.String(http.StatusNotFound, err.Error())
        } else {
            var b strings.Builder
            b.WriteString("Top playlists:")
            for _, item := range top.Playlists {
                b.WriteString("\n- ")
                b.WriteString(item.Name)
                b.WriteString(" : ")
                b.WriteString(item.Owner.ID)
            }
            endpoint = "/user"
            c.String(http.StatusOK, b.String())
        }
    })
    r.GET("/search", func(c *gin.Context) {
        query := c.DefaultQuery("q", "ABBA")
        searchCategory := c.DefaultQuery("c", "track")
        searchType := searchType(searchCategory)
        tok, tokenErr := getToken()
        if tokenErr != nil {
            log.Println("Not found token. Authenticating first")
            endpoint = fmt.Sprintf("/search?q=%s&c=%s", query, searchCategory)
            c.Request.URL.Path = "/auth"
            r.HandleContext(c)
            return
        }
        client = auth.NewClient(tok)
        results, err := client.Search(query, searchType)
        if err != nil {
            log.Println(err.Error())
            c.String(http.StatusNotFound, err.Error())
        }
        resString := handleSearchResults(results)
        endpoint = "/user"
        c.String(http.StatusOK, resString)
    })

    r.Run() // listen and serve on 0.0.0.0:8080`
}

func getToken() (*oauth2.Token, error)  {
    gc, found := kacha.Get("token")
    if found {
        log.Println("Found cached token")
        tok := gc.(*oauth2.Token)
        return tok, nil
    }
    log.Println("No cached token found. Looking for saved token")
    tok, err := tokenFromFile("./token.json")
    if err == nil {
        log.Println("Cached token")
        kacha.Set("token", tok, cache.DefaultExpiration)
    }
    return tok, err
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
    f, err := os.Open(file)
    defer f.Close()
    if err != nil {
        return nil, err
    }
    tok := &oauth2.Token{}
    err = json.NewDecoder(f).Decode(tok)
    return tok, err
}

func handleSearchResults(results *spotify.SearchResult) string {
    var b strings.Builder
    // handle album results
    if results.Albums != nil {
        b.WriteString("\nAlbums:\n")
        for _, item := range results.Albums.Albums {
            b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Artists[0].Name))
        }
    }
    // handle playlist results
    if results.Playlists != nil {
        b.WriteString("\nPlaylists:\n")
        for _, item := range results.Playlists.Playlists {
            b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Owner.DisplayName))
        }
    }
    // handle tracks results
    if results.Tracks != nil {
        b.WriteString("\nTracks:\n")
        for _, item := range results.Tracks.Tracks {
            b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Album.Name))
        }
    }
    // handle artists results
    if results.Artists != nil {
        b.WriteString("\nArtists:\n")
        for _, item := range results.Artists.Artists {
            b.WriteString(fmt.Sprintf("- %s : %s\n", item.Name, item.Popularity))
        }
    }
    return b.String()
}

func searchType(a string) spotify.SearchType {
    switch a {
        case "track":
            return spotify.SearchTypeTrack
        case "playlist":
            return spotify.SearchTypePlaylist
        case "album":
            return spotify.SearchTypeAlbum
        case "artist":
            return spotify.SearchTypeArtist
        default:
            return spotify.SearchTypeTrack
    }
}

