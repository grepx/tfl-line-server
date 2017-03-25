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
	"io/ioutil"
)

var (
	db     *sql.DB
)

func main() {
	log.Output(1, "gregz test logging")
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	//var err error

	//db, err = sql.Open("postgres", os.Getenv("DATABASE_URL") + " sslmode=disable")
	//if err != nil {
	//	log.Fatalf("Error opening database: %q", err)
	//}

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/test", test)
	router.GET("/getLineStatus", getLineStatus)
	router.GET("/updateLineStatus", updateLineStatus)

	router.Run(":" + port)
}

func test(c *gin.Context) {
	c.String(http.StatusOK, "test works")
}

func getLineStatus(c *gin.Context) {
	statusJson := getLineStatusFromDatabase()
	c.String(http.StatusOK, fmt.Sprintf("Read from database: %s\n", statusJson))
}

func getLineStatusFromDatabase() string {
	return "no value"
}

func decodeLineStatusJson(statusJson string) []LineStatus {
	byt := []byte(statusJson)
	lineStatus := make([]LineStatus,0)

	if err := json.Unmarshal(byt, &lineStatus); err != nil {
		panic(err)
	}
	return lineStatus
}

// fetch the latest line status and update the database
func updateLineStatus(c *gin.Context) {
	statusJson, err := fetchStatusJson()
	if (err == nil) {
		c.String(http.StatusOK, fmt.Sprintf("Read from network: %s\n", statusJson))
	}
}

func fetchStatusJson() (string, error) {
	url := "https://api.tfl.gov.uk/Line/Mode/tube%2Coverground%2Cdlr/Status?detail=true"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal("NewRequest: ", err)
		return "", err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Do: ", err)
		return "", err
	}

	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("ReadAll: ", err)
		return "", err
	}
	json := string(bytes)

	return json, nil
}

type LineStatus struct {
	Id string `json:"id"`
}

type LineStatusResponse struct {
	Lines []LineStatus
}