package main

import (
	"fmt"
	"log"
	"time"

	"github.com/boltdb/bolt"
	"github.com/chilts/rod"
	"github.com/garyburd/redigo/redis"
)

func incHits(pool *redis.Pool, id string) {
	if pool == nil {
		return
	}

	conn := pool.Get()
	defer conn.Close()

	fmt.Printf("incrementing hits for %s here\n", id)

	// do ALL times in UTC
	t := now()
	day := t.Format("20060102")
	hour := t.Format("15")
	fmt.Printf("time=%s\n", t)

	conn.Send("MULTI")
	conn.Send("INCR", "hits:"+id+":total")
	conn.Send("HINCRBY", "hits:"+id+":day", day, 1)
	conn.Send("HINCRBY", "hits:"+id+":hour", hour, 1)
	conn.Send("SADD", "active", id)
	_, err := conn.Do("EXEC")
	if err != nil {
		log.Printf("incHits: %s\n", err)
	}
}

func getTotalHits(pool *redis.Pool, id string) (int64, error) {
	if pool == nil {
		return 0, nil
	}

	conn := pool.Get()
	defer conn.Close()

	hits, err := redis.Int64(conn.Do("GET", "hits:"+id+":total"))
	fmt.Printf("hits=%d\n", hits)
	fmt.Printf("err=%s\n", err)
	if err == redis.ErrNil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return hits, nil
}

func stats(pool *redis.Pool, db *bolt.DB) {
	if pool == nil {
		log.Printf("Not setting up stats collection from Redis")
		return
	}

	// every 10s, hit redis for 'active' ShortURLs
	duration := time.Duration(10) * time.Second

	ticker := time.NewTicker(duration)
	for t := range ticker.C {
		log.Println("Tick at", t)
		statsRand(pool, db)
	}
}

func statsRand(pool *redis.Pool, db *bolt.DB) {
	conn := pool.Get()
	defer conn.Close()

	// firstly, get a random active Short URL
	ids, err := redis.Strings(conn.Do("SRANDMEMBER", "active", 1))
	if err != nil {
		log.Printf("statsRand: %s\n", err)
		return
	}
	if len(ids) == 0 {
		// no Short URLs are currently active
		return
	}
	id := ids[0]
	fmt.Printf("id=%s\n", id)

	// total
	total, err := redis.Int64(conn.Do("GET", "hits:"+id+":total"))
	if err != nil {
		log.Printf("statsRand: %s\n", err)
		return
	}
	fmt.Printf("total  = %d\n", total)

	// daily
	daily, err := redis.Int64Map(conn.Do("HGETALL", "hits:"+id+":day"))
	if err != nil {
		log.Printf("statsRand: %s\n", err)
		return
	}
	fmt.Printf("daily = %#v\n", daily)

	// hourly
	hourly, err := redis.Int64Map(conn.Do("HGETALL", "hits:"+id+":hour"))
	if err != nil {
		log.Printf("statsRand: %s\n", err)
		return
	}
	fmt.Printf("hourly = %#v\n", hourly)

	// put these stats into Bolt
	hit := Hit{
		Total:  total,
		Daily:  daily,
		Hourly: hourly,
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return rod.PutJson(tx, hitBucketNameStr, id, hit)
	})
	if err != nil {
		log.Printf("statsRand: %s\n", err)
	}
}
