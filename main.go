package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/abhinavdahiya/go-messenger-bot"
)

func main() {
	port := os.Getenv("PORT")
    access_token := os.Getenv("ACCESS_TOKEN")
    app_secret := os.Getenv("APP_SECRET")
    webhook_verify_token := os.Getenv("WEBHOOK_VERIFY_TOKEN")

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

	go router.Run(":" + port)

	for callback := range callbacks {
        log.Printf("[%#v] %s", callback.Sender, callback.Message.Text)

        msg := mbotapi.NewMessage(callback.Message.Text)
        bot.Send(callback.Sender, msg, mbotapi.RegularNotif)
    }
}
