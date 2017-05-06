package main

import (
	"log"
	"net/http"
	"os"
    "database/sql"
    "fmt"
    "strconv"

	"github.com/gin-gonic/gin"
	"github.com/abhinavdahiya/go-messenger-bot"
    _ "github.com/lib/pq"
)

var (
    db     *sql.DB
)

func dbFunc(c *gin.Context) {
	idString := c.Query("id")
	log.Printf("id=%s", idString)
	id, err := strconv.ParseInt(idString, 10, 64)
	if err == nil {
		if c.Query("delete") == "true" {
			deleteUser(id)
		} else {
			insertUser(id)
		}
	}

    rows, err := db.Query("SELECT id FROM users")
    if err != nil {
        c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error reading ids: %q", err))
        return
    }

    defer rows.Close()
    for rows.Next() {
        var id int64
        if err := rows.Scan(&id); err != nil {
          c.String(http.StatusInternalServerError,
            fmt.Sprintf("Error scanning id: %q", err))
            return
        }
        c.String(http.StatusOK, fmt.Sprintf("Read from DB: %d\n", id))
    }
}

func setupUsers() {
    // if _, err := db.Exec("DROP TABLE users"); err != nil {
    //     log.Printf("Error dropping database table: %q", err)
    // }
    if _, err := db.Exec("CREATE TABLE IF NOT EXISTS users (last_seen timestamp, id bigint, CONSTRAINT users_uq unique (id))"); err != nil {
        log.Printf("Error creating database table: %q", err)
    }
}

func insertUser(id int64) {
    if _, err := db.Exec("INSERT INTO users (last_seen, id) VALUES (now(), $1) ON CONFLICT (id) DO UPDATE SET last_seen=now()", id); err != nil {
        log.Printf("Error adding user: %q", err)
        return
    }
}
func deleteUser(id int64) {
    if _, err := db.Exec("DELETE FROM users WHERE id=$1", id); err != nil {
        log.Printf("Error deleting user: %q", err)
        return
    }
}

func forwardToUsers(bot *mbotapi.BotAPI, callback mbotapi.Callback) {
    log.Printf("[%#v] %s", callback.Sender, callback.Message.Text)
	if !callback.IsMessage() {
		log.Printf("'twas just an echo")
		return
    }

    insertUser(callback.Sender.ID)
    rows, err := db.Query("SELECT id FROM users")
    if err != nil {
        log.Printf("Error reading users: %q", err)
        return
    }
    defer rows.Close()

    msg := mbotapi.NewMessage(callback.Message.Text)
    for rows.Next() {
        var id int64
        if err := rows.Scan(&id); err != nil {
            log.Printf("Error scanning id: %q", err)
            return
        }
        user := mbotapi.NewUserFromID(id)
	    bot.Send(user, msg, mbotapi.RegularNotif)
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
	bot.Debug = true

	router.GET("/webhook", gin.WrapH(mux))
	router.POST("/webhook", gin.WrapH(mux))

    var err error

    db, err = sql.Open("postgres", database_url)
    if err != nil {
        log.Fatalf("Error opening database: %q", err)
    }
    router.GET("/db", dbFunc)

    setupUsers()

	go router.Run(":" + port)

	for callback := range callbacks {
		forwardToUsers(bot, callback)
    }
}
