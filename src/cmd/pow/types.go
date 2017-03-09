package main

import "time"

type ShortUrl struct {
	Id      string
	Url     string
	Created time.Time
	Updated time.Time
}
