package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/abhinavdahiya/go-messenger-bot"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

var (
	db  *sql.DB
	bot *mbotapi.BotAPI
)

func dbFunc(c *gin.Context) {
	idString := c.Query("id")
	log.Printf("id=%s", idString)
	id, err := strconv.ParseInt(idString, 10, 64)
	if err == nil {
		if c.Query("delete") == "true" {
			deleteUser(id)
		} else {
			fullUser, err := insertUser(id)
			if err != nil {
				fmt.Printf("Error reading full user: %q", err)
				return
			}
			c.String(http.StatusOK, fmt.Sprintf("Full User: %s %s\n", fullUser.FirstName, fullUser.LastName))
		}
	}

	rows, err := db.Query(`
		SELECT id FROM users
		WHERE last_seen > now() - '1 hour'::INTERVAL`)
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
	if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS users (
				last_seen timestamp, id bigint,
				CONSTRAINT users_uq UNIQUE (id)
			)`); err != nil {
		log.Printf("Error creating database table: %q", err)
	}
	if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS full_users (
				id bigint,
				first_name text,
				last_name text,
				profile_pic text,
				locale text,
				timezone bigint,
				gender text,
				CONSTRAINT full_users_uq UNIQUE (id)
			)`); err != nil {
		log.Printf("Error creating database table: %q", err)
	}
}

func insertUser(id int64) (mbotapi.FullUser, error) {
	fullUser := mbotapi.FullUser{User: mbotapi.NewUserFromID(id)}

	if _, err := db.Exec(`
			INSERT INTO users (last_seen, id)
			VALUES (now(), $1)
			ON CONFLICT (id) DO UPDATE SET last_seen=now()
			`, id); err != nil {
		log.Printf("Error adding user: %q", err)
		return fullUser, err
	}

	err := db.QueryRow(`
		SELECT (first_name, last_name, profile_pic, locale, timezone, gender)
		from full_users WHERE id=$1`, id).Scan(
		&fullUser.FirstName,
		&fullUser.LastName,
		&fullUser.ProfilePic,
		&fullUser.Locale,
		&fullUser.Timezone,
		&fullUser.Gender)

	if err == nil {
		return fullUser, nil
	} else if err != nil && err != sql.ErrNoRows {
		log.Fatalf("error checking if row exists %v", err)
		return fullUser, err
	} else /* if err == sql.ErrNoRows */ {
		fullUser, err := bot.GetFullUser(mbotapi.NewUserFromID(id))
		if err != nil {
			log.Fatalf("error fetching full user %v", err)
			return fullUser, err
		}

		_, err = db.Exec(`
				INSERT INTO full_users (id, first_name, last_name, profile_pic, locale, timezone, gender)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (id) DO NOTHING`,
			fullUser.User.ID,
			fullUser.FirstName,
			fullUser.LastName,
			fullUser.ProfilePic,
			fullUser.Locale,
			fullUser.Timezone,
			fullUser.Gender)

		if err != nil {
			log.Fatalf("error inserting full user %v", err)
			return fullUser, err
		} else {
			return fullUser, nil
		}
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

	fullUser, err := insertUser(callback.Sender.ID)
	if err != nil {
		log.Printf("Error getting full user: %q", err)
		return
	}
	rows, err := db.Query(`
		SELECT id FROM users
		WHERE last_seen > now() - '1 hour'::INTERVAL`)
	if err != nil {
		log.Printf("Error reading users: %q", err)
		return
	}
	defer rows.Close()

	msg := mbotapi.NewMessage(
		fmt.Sprintf(
			"%s %s: %s", fullUser.FirstName, fullUser.LastName, callback.Message.Text))
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

	bot = mbotapi.NewBotAPI(access_token, webhook_verify_token, app_secret)
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
