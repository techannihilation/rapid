package main

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(sessions.Sessions("admin-session", store))
	r.LoadHTMLGlob("templates/*")
	r.GET("/repos.gz", ReposHandler)
	r.GET("/:shortname/versions.gz", VersionsHandler)
	r.GET("/:shortname/packages/:filename", PackageHandler)
	r.POST("/:shortname/streamer.cgi", StreamerHandler)

	admin := r.Group("/admin")
	{
		admin.GET("/login", ShowLogin)
		admin.POST("/login", HandleLogin)

		admin.GET("/logout", Logout)

		protected := admin.Group("/")
		protected.Use(AuthMiddleware(DB))
		{
			protected.GET("/", Dashboard)

			protected.GET("/games", ListGames)
			protected.GET("/games/new", ShowNewGame)
			protected.POST("/games", CreateGame)

			protected.GET("/games/:id/versions", ListVersions)
			protected.POST("/versions/:id/togglepublish", TogglePublishVersion)
		}
	}

	return r
}

func AuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, _ := store.Get(c.Request, "admin-session")
		adminID := session.Values["admin_id"]

		if adminID == nil {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		c.Next()
	}
}
