package main

import (
	"log"
	"net/http"
	"os"
    "database/sql"
    "fmt"
    "time"

	"github.com/gin-gonic/gin"
	"github.com/abhinavdahiya/go-messenger-bot"
    _ "github.com/lib/pq"
)

var (
    db     *sql.DB
)

func dbFunc(c *gin.Context) {
    if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ticks (tick timestamp)"); err != nil {
        c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error creating database table: %q", err))
        return
    }

    if _, err := db.Exec("INSERT INTO ticks VALUES (now())"); err != nil {
        c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error incrementing tick: %q", err))
        return
    }

    rows, err := db.Query("SELECT tick FROM ticks")
    if err != nil {
        c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error reading ticks: %q", err))
        return
    }

    defer rows.Close()
    for rows.Next() {
        var tick time.Time
        if err := rows.Scan(&tick); err != nil {
          c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error scanning ticks: %q", err))
            return
        }
        c.String(http.StatusOK, fmt.Sprintf("Read from DB: %s\n", tick.String()))
    }
}


func main() {
	port := os.Getenv("PORT")
    access_token := os.Getenv("ACCESS_TOKEN")
    app_secret := os.Getenv("APP_SECRET")
    webhook_verify_token := os.Getenv("WEBHOOK_VERIFY_TOKEN")
    database_url := os.Getenv("DATABASE_URL")

	if port == "" {
		log.Fatal("$PORT must be set")
	}
	if access_token == "" {
		log.Fatal("$ACCESS_TOKEN must be set")
	}
	if app_secret == "" {
		log.Fatal("$APP_SECRET must be set")
	}
	if webhook_verify_token == "" {
		log.Fatal("$WEBHOOK_VERIFY_TOKEN must be set")
	}
	if database_url == "" {
		log.Fatal("$DATABASE_URL must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.LoadHTMLGlob("templates/*.tmpl.html")
	router.Static("/static", "static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})

	bot := mbotapi.NewBotAPI(access_token, webhook_verify_token, app_secret)
	callbacks, mux := bot.SetWebhook("/webhook")

	router.GET("/webhook", gin.WrapH(mux))
	router.POST("/webhook", gin.WrapH(mux))

    var err error

    db, err = sql.Open("postgres", database_url)
    if err != nil {
        log.Fatalf("Error opening database: %q", err)
    }
    router.GET("/db", dbFunc)

	go router.Run(":" + port)

	for callback := range callbacks {
        log.Printf("[%#v] %s", callback.Sender, callback.Message.Text)

        msg := mbotapi.NewMessage(callback.Message.Text)
        bot.Send(callback.Sender, msg, mbotapi.RegularNotif)
    }
}
