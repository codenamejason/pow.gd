package main

import (
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/boltdb/bolt"
	"github.com/chilts/rod"
	"github.com/gomiddleware/mux"
)

var urlBucketName = []byte("url")
var urlBucketNameStr = "url"

const idChars string = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const idCharLen = len(idChars)

func Id() string {
	str := ""
	for i := 0; i < 8; i++ {
		r := rand.Intn(idCharLen)
		str = str + string(idChars[r])
	}
	return str
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// setup
	baseUrl := os.Getenv("POW_BASE_URL")
	port := os.Getenv("POW_PORT")
	if port == "" {
		log.Fatal("Specify a port to listen on in the environment variable 'POW_PORT'")
	}

	// load up all templates
	tmpl, err := template.New("").ParseGlob("./templates/*.html")
	if err != nil {
		log.Fatal(err)
	}

	// open the datastore
	db, err := bolt.Open("pow.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// create the main url bucket
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(urlBucketName)
		return err
	})

	// the mux
	m := mux.New()

	// do some static routes before doing logging
	m.All("/s", fileServer("static"))
	m.Get("/favicon.ico", serveFile("./static/favicon.ico"))
	m.Get("/robots.txt", serveFile("./static/robots.txt"))

	m.Get("/", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			BaseUrl string
		}{
			baseUrl,
		}
		render(w, tmpl, "index.html", data)
	})

	m.Post("/", func(w http.ResponseWriter, r *http.Request) {
		url := r.FormValue("url")

		// setup a few things
		var id string
		now := time.Now().UTC()
		shortUrl := ShortUrl{
			Id:       "", // filled in later
			Url:      url,
			Inserted: now,
			Updated:  now,
		}

		// ToDo: validate the URL, until then, just put it into the right bucket

		err := db.Update(func(tx *bolt.Tx) error {
			// keep generating IDs until we find a unique one
			for {
				// generate a new Id
				id = Id()
				fmt.Printf("id=%s\n", id)

				// see if it already exists
				v, err := rod.Get(tx, urlBucketNameStr, id)
				if err != nil {
					return err
				}
				if v == nil {
					// this id does not yet exist, so quite the loop
					break
				}
				// ID exists, loop again ...
			}

			shortUrl.Id = id
			return rod.PutJson(tx, urlBucketNameStr, id, shortUrl)
		})

		if err != nil {
			internalServerError(w, err)
			return
		}

		fmt.Fprintf(w, "id=%s\n", shortUrl.Id)
		fmt.Fprintf(w, "url=%s\n", url)
	})

	m.Get("/:id", func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vals(r)["id"]
		fmt.Printf("id=%s\n", id)

		// get the shortUrl if it exists
		var shortUrl *ShortUrl
		err := db.View(func(tx *bolt.Tx) error {
			return rod.GetJson(tx, urlBucketNameStr, id, &shortUrl)
		})

		if err != nil {
			internalServerError(w, err)
			return
		}

		if shortUrl == nil {
			notFound(w, r)
			return
		}

		fmt.Printf("shortUrl=%#v\n", shortUrl)
		http.Redirect(w, r, shortUrl.Url, http.StatusMovedPermanently)
	})

	// finally, check all routing was added correctly
	check(m.Err)

	// server
	log.Printf("Starting server, listening on port %s\n", port)
	errServer := http.ListenAndServe(":"+port, m)
	check(errServer)
}
