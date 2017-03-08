package main

import "time"

type ShortUrl struct {
	Id       string
	Url      string
	Inserted time.Time
	Updated  time.Time
}
