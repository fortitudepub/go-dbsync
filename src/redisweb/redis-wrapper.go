package main

import (
	"../myutil"
	"github.com/go-redis/redis"
	"strconv"
	"time"
)

func newRedisClient(server RedisServer) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     server.Addr,
		Password: server.Password, // no password set
		DB:       server.DB,       // use default DB
	})
}

func redisInfo(server RedisServer) string {
	client := newRedisClient(server)
	defer client.Close()

	info, _ := client.Info().Result()
	return info
}

func configGetDatabases(server RedisServer) int {
	client := newRedisClient(server)
	defer client.Close()

	config, _ := client.ConfigGet("databases").Result()
	databaseNum, _ := strconv.Atoi(config[1].(string))
	return databaseNum
}

func changeContent(server RedisServer, key, content, format string) string {
	client := newRedisClient(server)
	defer client.Close()

	if format == "UNKNOWN" {
		content, _ = strconv.Unquote(content)
	}

	err := client.Set(key, content, 0).Err()
	if err == nil {
		return "OK"
	}

	return err.Error()
}

func newKey(server RedisServer, keyType, key, ttl, format, val string) string {
	client := newRedisClient(server)
	defer client.Close()

	var err error
	if format == "Quoted" {
		val, err = strconv.Unquote(val)
		if err != nil {
			return err.Error()
		}
	}

	var duration time.Duration = -1
	if ttl != "-1s" && ttl != "" {
		duration, err = time.ParseDuration(ttl)
		if err != nil {
			return err.Error()
		}
	}

	switch keyType {
	case "string":
		_, err = client.Set(key, val, duration).Result()

	}

	if err != nil {
		return err.Error()
	}

	return "OK"

}

func deleteKey(server RedisServer, key string) string {
	client := newRedisClient(server)
	defer client.Close()

	ok, err := client.Del(key).Result()
	if ok == 1 {
		return "OK"
	} else {
		return err.Error()
	}
}

type ContentResult struct {
	Exists   bool
	Content  interface{}
	Ttl      string
	Encoding string
	Size     int64
	Error    string
	Format   string // JSON, NORMAL, UNKNOWN
}

func displayContent(server RedisServer, key string, valType string) *ContentResult {
	client := newRedisClient(server)
	defer client.Close()

	exists, _ := client.Exists(key).Result()
	if exists == 0 {
		return &ContentResult{
			Exists:   false,
			Content:  "",
			Ttl:      "",
			Encoding: "",
			Size:     0,
			Error:    "",
		}
	}

	var errorMessage string
	ttl, _ := client.TTL(key).Result()
	encoding, _ := client.ObjectEncoding(key).Result()
	var content interface{}
	var format string
	var err error
	var size int64

	switch valType {
	case "string":
		size, _ = client.StrLen(key).Result()
		content, err = client.Get(key).Result()
		if err == nil {
			content, format = parseStringFormat(content.(string))
		}

	case "hash":
		content, err = client.HGetAll(key).Result()
		size, _ = client.HLen(key).Result()
		content = parseHashContent(content.(map[string]string))
	case "list":
		content, err = client.LRange(key, 0, -1).Result()
		size, _ = client.LLen(key).Result()
	case "set":
		content, err = client.SMembers(key).Result()
		size, _ = client.SCard(key).Result()
	case "zset":
		content, err = client.ZRangeWithScores(key, 0, -1).Result()
		size, _ = client.ZCard(key).Result()
	default:
		content = "unknown type " + valType
	}

	if err != nil {
		errorMessage = err.Error()
	}

	return &ContentResult{
		Exists:   true,
		Content:  content,
		Ttl:      ttl.String(),
		Encoding: encoding,
		Size:     size,
		Error:    errorMessage,
		Format:   format,
	}
}
func parseHashContent(m map[string]string) map[string]string {
	converted := make(map[string]string, len(m))
	for k, v := range m {
		ck := convertString(k)
		cv := convertString(v)
		converted[ck] = cv
	}

	return converted
}

func convertString(s string) string {
	if s == "" || myutil.IsPrintable(s) {
		return s
	}

	return strconv.Quote(s)
}

func parseStringFormat(s string) (string, string) {
	if s == "" {
		return s, "UNKNOWN"
	}

	if myutil.IsJSON(s) {
		return myutil.JSONPrettyPrint(s), "JSON"
	}

	if myutil.IsPrintable(s) {
		return s, "NORMAL"
	}

	return strconv.Quote(s), "UNKNOWN"
}

type KeysResult struct {
	Key  string
	Type string
	Len  int64
}

func listKeys(server RedisServer, matchPattern string, maxKeys int) ([]KeysResult, error) {
	client := newRedisClient(server)
	defer client.Close()

	allKeys := make([]KeysResult, 0)
	var keys []string
	var cursor uint64
	var err error

	for {
		keys, cursor, err = client.Scan(cursor, matchPattern, 10).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			valType, err := client.Type(key).Result()
			if err != nil {
				return nil, err
			}

			var len int64
			switch valType {
			case "list":
				len, _ = client.LLen(key).Result()
			case "hash":
				len, _ = client.HLen(key).Result()
			case "set":
				len, _ = client.SCard(key).Result()
			case "zset":
				len, _ = client.ZCard(key).Result()
			default:
				len = 1
			}

			allKeys = append(allKeys, KeysResult{Key: key, Type: valType, Len: len})
		}

		if cursor == 0 || len(allKeys) >= maxKeys {
			break
		}
	}

	return allKeys, nil
}
