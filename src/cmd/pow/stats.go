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

	// get a connection
	conn := pool.Get()
	defer conn.Close()

	fmt.Printf("incrementing hits for %s here\n", id)

	// do ALL times in UTC
	datetime := now().Format("20060102-15")

	// just inc count:20060102-15:<id>
	conn.Send("MULTI")
	conn.Send("INCR", "count:"+datetime+":"+id)
	conn.Send("SADD", "active:"+datetime, id)
	_, err := conn.Do("EXEC")
	if err != nil {
		log.Printf("incHits: %s\n", err)
	}
}

func stats(pool *redis.Pool, db *bolt.DB) {
	if pool == nil {
		log.Printf("Not setting up stats collection from Redis")
		return
	}

	// every 15s, hit redis for 'active' ShortURLs
	duration := time.Duration(15) * time.Second

	ticker := time.NewTicker(duration)
	for t := range ticker.C {
		log.Println("Tick at", t)
		processRandStat(pool, db)
	}
}

func processRandStat(pool *redis.Pool, db *bolt.DB) {
	// get a connection
	conn := pool.Get()
	defer conn.Close()

	// do ALL times in UTC
	t := now().Add(-60 * time.Minute)
	datetime := t.Format("20060102-15")

	fmt.Printf("Looking for an active ID in the previous hour ...\n")

	// get one random ID
	id, err := redis.String(conn.Do("SRANDMEMBER", "active:"+datetime))
	if err != nil {
		log.Printf(err.Error())
		return
	}

	fmt.Printf("* id=%s\n", id)

	// get it's hit count
	hour := datetime + ":" + id
	count, err := redis.Int64(conn.Do("GET", "count:"+hour))
	if err != nil {
		log.Printf(err.Error())
		return
	}
	fmt.Printf("* count=%d\n", count)

	// put these stats into Bolt
	stats := Stats{}
	err = db.Update(func(tx *bolt.Tx) error {
		// firstly, let's see if these stats have already been processed
		done, err := rod.GetString(tx, doneBucketNameStr, hour)
		if err != nil {
			return err
		}

		if done == "" {
			fmt.Printf("not yet done\n")
			// get the stats and increment the right slots
			fmt.Printf("* 1 stats=%#v\n", stats)
			rod.GetJson(tx, statsBucketNameStr, id, &stats)
			fmt.Printf("* 2 stats=%#v\n", stats)
			stats.Total += count
			if stats.Daily == nil {
				stats.Daily = make(map[string]int64)
			}
			stats.Daily[t.Format("20060102")] += count
			if stats.Hourly == nil {
				stats.Hourly = make(map[string]int64)
			}
			stats.Hourly[t.Format("15")] += count
			if stats.DOTWly == nil {
				stats.DOTWly = make(map[string]int64)
			}
			stats.DOTWly[t.Format("Mon")] += count
			fmt.Printf("* 3 stats=%#v\n", stats)
			//
			err = rod.PutJson(tx, statsBucketNameStr, id, stats)
			if err != nil {
				return err
			}
			// and say we're done
			err = rod.PutString(tx, doneBucketNameStr, hour, now().Format("20060102-150405.000000000"))
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("done:%s\n", done)
		}

		return nil
	})
	if err != nil {
		log.Printf(err.Error())
	}

	// and finally, remove this hit from Redis
	conn.Send("MULTI")
	conn.Send("DEL", "count:"+hour)
	conn.Send("SREM", "active:"+datetime, id)
	_, err = conn.Do("EXEC")
	if err != nil {
		log.Printf("incHits: %s\n", err)
	}

	// ToDo: check if the "active:"+datetime now has zero members, and if so, remove the key
}
