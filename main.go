package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"encoding/json"
)

var (
	db     *sql.DB
)

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	var err error

	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL") + " sslmode=disable")
	if err != nil {
		log.Fatalf("Error opening database: %q", err)
	}

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/updateLines", updateLines)

	router.Run(":" + port)
}

func updateLines(c *gin.Context) {
	response := doNetworkCall()
	for i := 0; i < 10; i++ {
		c.String(http.StatusOK, fmt.Sprintf("Read from network: %s\n", response[i].Id))
	}
	return
}

type LineStatus struct {
	Id string `json:"id"`
}

type LineStatusResponse struct {
	Lines []LineStatus
}

func doNetworkCall() []LineStatus {
	lineStatus := make([]LineStatus, 0)

	url := "https://api.tfl.gov.uk/Line/Mode/tube%2Coverground%2Cdlr/Status?detail=true"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal("NewRequest: ", err)
		return lineStatus
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Do: ", err)
		return lineStatus
	}

	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&lineStatus); err != nil {
		log.Println(err)
	}
	return lineStatus
}