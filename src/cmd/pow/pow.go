package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/chilts/rod"
	"github.com/garyburd/redigo/redis"
	"github.com/gomiddleware/logger"
	"github.com/gomiddleware/logit"
	"github.com/gomiddleware/mux"
)

var urlBucketName = []byte("url")
var urlBucketNameStr = "url"
var statsBucketName = []byte("stats")
var statsBucketNameStr = "stats"
var doneBucketName = []byte("done") // as in "stats-done"
var doneBucketNameStr = "done"      // as in "stats-done"

var (
	ErrInvalidScheme            = errors.New("URL scheme must be http or https")
	ErrInvalidHost              = errors.New("Host must contain letters/numbers, contain at least one dot and last component is at least 2 letters")
	ErrHostCantHaveDashesHere   = errors.New("Host can't have dashes next to dots anywhere")
	ErrHostCantBeginEndWithDash = errors.New("Host can't begin or end with a dash")
)

var domainRegExp = regexp.MustCompile(`^([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}$`)
var invalidDashRegExp = regexp.MustCompile(`(\.-)|(-\.)`)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func validateUrl(str string) (*url.URL, error) {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return u, err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return u, ErrInvalidScheme
	}

	u.Host = strings.ToLower(u.Host)
	if !domainRegExp.MatchString(u.Host) {
		return u, ErrInvalidHost
	}

	// see if we match any of '.-' or '-.
	if invalidDashRegExp.MatchString(u.Host) {
		return u, ErrHostCantHaveDashesHere
	}

	// or if it starts or ends with a dash
	if strings.HasPrefix(u.Host, "-") || strings.HasSuffix(u.Host, "-") {
		return u, ErrHostCantBeginEndWithDash
	}

	return u, nil
}

func main() {
	// setup the logger
	lgr := logit.New(os.Stdout, "pow")

	// setup
	nakedDomain := os.Getenv("POW_NAKED_DOMAIN")
	baseUrl := os.Getenv("POW_BASE_URL")
	port := os.Getenv("POW_PORT")
	if port == "" {
		log.Fatal("Specify a port to listen on in the environment variable 'POW_PORT'")
	}

	// load up all templates
	tmpl, err := template.New("").ParseGlob("./templates/*.html")
	check(err)

	// connect to Redis if specified
	var redisPool *redis.Pool
	redisAddr := os.Getenv("POW_REDIS_ADDR")
	lgr.Print("redis-addr")
	if redisAddr != "" {
		fmt.Println("Configuring Redis")
		redisPool = &redis.Pool{
			MaxIdle:     3,
			IdleTimeout: 240 * time.Second,
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", redisAddr)
			},
		}
		lgr.Print("redis-configured")
	} else {
		fmt.Println("Not configuring Redis, hit counts will not function")
		lgr.Print("redis-not-configured")
	}

	// open the datastore
	db, err := bolt.Open("pow.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	check(err)
	defer db.Close()

	// create the main buckets
	err = db.Update(func(tx *bolt.Tx) error {
		var err error

		urlBucket, err := tx.CreateBucketIfNotExists(urlBucketName)
		if err != nil {
			return err
		}

		err = urlBucket.Delete([]byte("/RToXsy"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(statsBucketName)
		if err != nil {
			return err
		}

		return nil
	})
	check(err)

	// Run the stats at regular intervals to process the hits from the previous hour.
	go stats(redisPool, db)

	// the mux
	m := mux.New()

	m.Use("/", logger.NewLogger(lgr))

	// do some static routes before doing logging
	m.All("/s", fileServer("static"))
	m.Get("/favicon.ico", serveFile("./static/favicon.ico"))
	m.Get("/robots.txt", serveFile("./static/robots.txt"))

	m.Get("/", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			NakedDomain string
			BaseUrl     string
		}{
			nakedDomain,
			baseUrl,
		}
		render(w, tmpl, "index.html", data)
	})

	m.Get("/new", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			BaseUrl string
		}{
			baseUrl,
		}
		render(w, tmpl, "new.html", data)
	})

	m.Post("/new", func(w http.ResponseWriter, r *http.Request) {
		// validate the URL
		u, err := validateUrl(r.FormValue("url"))
		if err != nil {
			internalServerError(w, err)
			return
		}

		fmt.Printf("url=%s\n", u)

		// setup a few things
		var id string
		now := time.Now().UTC()
		shortUrl := ShortUrl{
			Id:      "", // filled in later
			Url:     u.String(),
			Created: now,
			Updated: now,
		}

		err = db.Update(func(tx *bolt.Tx) error {
			// keep generating IDs until we find a unique one
			for {
				// generate a new Id
				id = Id(6)
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

		http.Redirect(w, r, "/"+id+"+", http.StatusFound)
	})

	m.Get("/:id", func(w http.ResponseWriter, r *http.Request) {
		var preview bool
		id := mux.Vals(r)["id"]
		fmt.Printf("id=%s\n", id)

		lgr := logger.LogFromRequest(r)
		lgr.WithField("ShortUrlId", id)

		// decide if we're redirecting or viewing the preview page (https://play.golang.org/p/Mkpb9gAzN1)
		if strings.HasSuffix(id, "+") {
			id = strings.TrimSuffix(id, "+")
			preview = true
		}

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
			lgr.Print("no-short-url-found")
			notFound(w, r)
			return
		}

		if preview {
			// get the stats (if it exists)
			stats := Stats{}
			err := db.View(func(tx *bolt.Tx) error {
				return rod.GetJson(tx, statsBucketNameStr, id, &stats)
			})
			if err != nil {
				internalServerError(w, err)
				return
			}
			fmt.Printf("stats=%#v\n", stats)

			lgr.Print("rendering-preview")
			data := struct {
				BaseUrl  string
				ShortUrl *ShortUrl
				Stats    *Stats
			}{
				baseUrl,
				shortUrl,
				&stats,
			}
			render(w, tmpl, "preview.html", data)
		} else {
			go incHits(redisPool, id)
			http.Redirect(w, r, shortUrl.Url, http.StatusMovedPermanently)
		}
	})

	// finally, check all routing was added correctly
	check(m.Err)

	// server
	fmt.Printf("Starting server, listening on port %s\n", port)
	errServer := http.ListenAndServe(":"+port, m)
	check(errServer)
}
