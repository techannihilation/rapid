package main

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
)

var store cookie.Store

func main() {
	cfg, err := LoadConfig()
	store = cookie.NewStore([]byte(cfg.CookieSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
	})
	if err != nil {
		panic(err)
	}
	InitDB(cfg)
	StartGitPoller(cfg)

	r := SetupRouter()
	r.Run(":8080")
}
