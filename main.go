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
	firebaseKey string
)

func main() {
	log.Output(1, "gregz test logging")
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	var err error

	// open database connection
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL") + " sslmode=disable")
	if err != nil {
		log.Fatalf("Error opening database: %q", err)
		panic(err)
	}

	// load firebase key
	firebaseKey = os.Getenv("FIREBASE_KEY")

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/getLineStatus", getLineStatus)
	router.GET("/updateLineStatus", updateLineStatus)

	router.Run(":" + port)
}

func getLineStatus(c *gin.Context) {
	linesJson, err := getLinesFromDatabase(c)
	if (err != nil) {
		return
	}
	printLines(c, linesJson)
}

func printLines(c *gin.Context, linesJson string) {
	lines := decodeLinesJson(linesJson)
	for i := 0; i < len(lines); i++ {
		c.String(http.StatusOK,
			fmt.Sprintf("\n%s:\n", lines[i].Id))
		printStatuses(c, lines[i].LineStatuses)
	}
}

func printStatuses(c *gin.Context, lineStatus []LineStatus) {
	for i := 0; i < len(lineStatus); i++ {
		c.String(http.StatusOK,
			fmt.Sprintf("%s\n", lineStatus[i].StatusSeverityDescription))
	}
}

func getLinesFromDatabase(c *gin.Context) (string, error) {
	rows, err := db.Query("SELECT json FROM status")
	if err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("Couldn't load from database, perhaps it isn't created yet? err: %s", err))
		return "", err
	}

	defer rows.Close()
	var linesJson string
	for rows.Next() {
		if err := rows.Scan(&linesJson); err != nil {
			c.String(http.StatusInternalServerError,
				fmt.Sprintf("Error reading json record. err: %s", err))
			return "", err
		}
	}
	// there should only be 1 record in the database
	return linesJson, nil
}

func decodeLinesJson(linesJson string) []Line {
	byt := []byte(linesJson)
	lines := make([]Line, 0)

	if err := json.Unmarshal(byt, &lines); err != nil {
		panic(err)
	}
	return lines
}

func updateLineStatus(c *gin.Context) {
	// fetch latest status
	linesJson, err := fetchLinesJson()
	if (err != nil) {
		return
	}
	// update database
	insertInDatabase(c, linesJson)

	printLines(c, linesJson)

	// send notification
	//lines := decodeLinesJson(linesJson)

}

func fetchLinesJson() (string, error) {
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

func insertInDatabase(c *gin.Context, linesJson string) {
	// create the table for the first time it doesn't exist
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS status (json text)"); err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("database error: %q", err))
		return
	}
	// delete the last record if the table already existed
	if _, err := db.Exec("DELETE FROM status"); err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("database error: %q", err))
		return
	}
	// yep, I'm inserting json into a database, no idea how else to store it on heroku, open to suggestions
	if _, err := db.Exec("INSERT INTO status VALUES ($1)", linesJson); err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("database error: %q", err))
		return
	}
}

func sendLineStatusNotification(lineId string, status string) (string, error) {
	url := "https://fcm.googleapis.com/fcm/send"

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Fatal("NewRequest: ", err)
		return "", err
	}

	req.Header.Add("Authorization", "key=" + firebaseKey)
	req.Header.Add("Content-Type", "application/json")
	
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

type Line struct {
	Id           string `json:"id"`
	LineStatuses []LineStatus `json:"lineStatuses"`
}

type LineStatus struct {
	Id                        int `json:"id"`
	StatusSeverityDescription string `json:"statusSeverityDescription"`
}