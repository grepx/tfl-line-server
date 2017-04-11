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
	"strconv"
	"github.com/NaySoftware/go-fcm"
	"time"
	"errors"
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
	enableDatabaseSsl, err := strconv.ParseBool(os.Getenv("ENABLE_DATABASE_SSL"))
	if (err != nil) {
		log.Fatalf("Config error %q", err)
		panic(err)
	}

	databaseUrl := os.Getenv("DATABASE_URL")
	if (!enableDatabaseSsl) {
		databaseUrl += " sslmode=disable"
	}

	db, err = sql.Open("postgres", databaseUrl)
	if err != nil {
		log.Fatalf("Error opening database: %q", err)
		panic(err)
	}

	// load firebase key
	firebaseKey = os.Getenv("FIREBASE_KEY")

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/startService", startService)

	router.Run(":" + port)
}

func startService(c *gin.Context) {
	// todo: add api key code
	// just spawn a timer that keeps calling on a go channel every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	startTime := time.Now().UTC().String()
	quit := make(chan struct{})
	count := 1

	go func() {
		for {
			select {
			case <-ticker.C:
				pollLineStatus(startTime, count * 10)
			// run it for 10 minutes and 10 seconds
			// by which point the next service poll has probably kicked in or will soon
				if (count >= 62) {
					close(quit)
				}
				count += 1
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	// return right away with success
	c.String(http.StatusOK, "service started")
}

func pollLineStatus(startTime string, secondsCount int) {
	logMessage :=
		fmt.Sprintf("Polling for latest line status. Seconds: %d, Start Time: %q",
			secondsCount, startTime)
	log.Output(1, logMessage)
}

func decodeLinesJson(linesJson string) []Line {
	byt := []byte(linesJson)
	lines := make([]Line, 0)

	if err := json.Unmarshal(byt, &lines); err != nil {
		panic(err)
	}
	return lines
}

func updateLineStatus() {
	// fetch latest status
	currentLinesJson, err := fetchLinesJson()
	if (err != nil) {
		return
	}

	oldLinesJson, err := getLinesFromDatabase()
	if (err != nil) {
		return
	}

	// send notification
	currentLines := decodeLinesJson(currentLinesJson)
	oldLines := decodeLinesJson(oldLinesJson)

	compareAndNotify(oldLines, currentLines)

	// update database
	insertInDatabase(currentLinesJson)
}

func compareAndNotify(oldLines []Line, currentLines []Line) {
	//for _, line := range oldLines {
	// todo
	//}
}

func getLineWithId(lines []Line, id string) (Line, error) {
	for _, line := range lines {
		if (line.Id == id) {
			return line, nil
		}
	}
	return Line{}, errors.New("no line with that id")
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

func insertInDatabase(linesJson string) {
	// create the table for the first time it doesn't exist
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS status (json text)"); err != nil {
		log.Output(1, fmt.Sprintf("database error: %q", err))
		return
	}
	// delete the last record if the table already existed
	if _, err := db.Exec("DELETE FROM status"); err != nil {
		log.Output(1, fmt.Sprintf("database error: %q", err))
		return
	}
	// yep, I'm inserting json into a database, no idea how else to store it on heroku, open to suggestions
	if _, err := db.Exec("INSERT INTO status VALUES ($1)", linesJson); err != nil {
		log.Output(1, fmt.Sprintf("database error: %q", err))
		return
	}
}

func getLinesFromDatabase() (string, error) {
	rows, err := db.Query("SELECT json FROM status")
	if err != nil {
		log.Output(1, fmt.Sprintf("Couldn't load from database, perhaps it isn't created yet? err: %s", err))
		return "", err
	}

	defer rows.Close()
	var linesJson string
	for rows.Next() {
		if err := rows.Scan(&linesJson); err != nil {
			log.Output(1, fmt.Sprintf("Error reading json record. err: %s", err))
			return "", err
		}
	}
	// there should only be 1 record in the database
	return linesJson, nil
}

type Line struct {
	Id           string `json:"id"`
	LineStatuses []LineStatus `json:"lineStatuses"`
}

type LineStatus struct {
	Id                        int `json:"id"`
	StatusSeverityDescription string `json:"statusSeverityDescription"`
}

func sendStatusNotification(lineName string, lineStatus string) {
	data := map[string]string{
		"msg": lineName + " status: " + lineStatus,
		"sum": "Happy Day",
	}

	c := fcm.NewFcmClient(firebaseKey)
	c.NewFcmMsgTo("/topics/" + lineName, data)

	status, err := c.Send()

	if err == nil {
		status.PrintResults()
	} else {
		fmt.Println(err)
	}
}