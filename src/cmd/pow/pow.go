package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gomiddleware/mux"
)

var urlBucketName = []byte("url")

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
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

		// ToDo: validate the URL, until then, just put it into the right bucket
		err := db.Update(func(tx *bolt.Tx) error {
			// get the bucket and it's next sequence number
			b := tx.Bucket(urlBucketName)
			id, err := b.NextSequence()
			if err != nil {
				return err
			}

			fmt.Printf("id=%d\n", id)

			return err
		})

		if err != nil {
			internalServerError(w, err)
		}

		fmt.Fprintf(w, "url=%s\n", url)
	})

	// finally, check all routing was added correctly
	check(m.Err)

	// server
	log.Printf("Starting server, listening on port %s\n", port)
	errServer := http.ListenAndServe(":"+port, m)
	check(errServer)
}
