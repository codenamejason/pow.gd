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

	// every 10s, hit redis for 'active' ShortURLs
	// duration := time.Duration(10) * time.Second
	duration := time.Duration(3) * time.Second

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

func statsOld(pool *redis.Pool, db *bolt.DB) {
	if pool == nil {
		log.Printf("Not setting up stats collection from Redis")
		return
	}

	// get a connection
	conn := pool.Get()
	defer conn.Close()

	// get all of the 'active' IDs
	active, err := redis.Strings(conn.Do("SMEMBERS", "active"))
	if err != nil {
		log.Printf(err.Error())
		return
	}
	for _, id := range active {
		fmt.Printf("active=%s\n", id)
		statsRandOld(pool, db, id)
	}
}

func statsRandOld(pool *redis.Pool, db *bolt.DB, id string) {
	// get a connection
	conn := pool.Get()
	defer conn.Close()

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

	// dotwly - day of the week(ly)
	dotwly, err := redis.Int64Map(conn.Do("HGETALL", "hits:"+id+":dotw"))
	if err != nil {
		log.Printf("statsRand: %s\n", err)
		return
	}
	fmt.Printf("dotwly = %#v\n", dotwly)

	// put these stats into Bolt
	stats := Stats{
		Total:  total,
		Daily:  daily,
		Hourly: hourly,
		DOTWly: dotwly,
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return rod.PutJson(tx, statsBucketNameStr, id, stats)
	})
	if err != nil {
		log.Printf("statsRand: %s\n", err)
	}

	// finally, remove the keys used here, and remove it from the original set
	// just inc count:20060102-15:<id>
	conn.Send("MULTI")
	conn.Send("DEL", "hits:"+id+":total")
	conn.Send("DEL", "hits:"+id+":day")
	conn.Send("DEL", "hits:"+id+":hour")
	conn.Send("DEL", "hits:"+id+":dotw")
	conn.Send("SREM", "active", id)
	_, err = conn.Do("EXEC")
	if err != nil {
		log.Printf("incHits: %s\n", err)
	}
}
