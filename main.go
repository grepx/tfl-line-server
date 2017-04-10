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

	router.GET("/getLineStatus", getLineStatus)
	router.GET("/updateLineStatus", updateLineStatus)
	router.GET("/sendPushNotification", sendPushNotification)

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

	c.String(http.StatusOK, "--- Previous line status ---")
	getLineStatus(c)

	// send notification
	//lines := decodeLinesJson(linesJson)

	// update database
	insertInDatabase(c, linesJson)

	c.String(http.StatusOK, "\n--- Updated line status ---")
	printLines(c, linesJson)
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

type Line struct {
	Id           string `json:"id"`
	LineStatuses []LineStatus `json:"lineStatuses"`
}

type LineStatus struct {
	Id                        int `json:"id"`
	StatusSeverityDescription string `json:"statusSeverityDescription"`
}

func sendPushNotification(c *gin.Context) {
	ticker := time.NewTicker(15 * time.Second)
	startTime := time.Now().UTC().String()
	quit := make(chan struct{})
	count := 0
	go func() {
		for {
			select {
			case <-ticker.C:
			//sendStatusNotification("northern", "no service" + strconv.Itoa(count))
				count += 1
				logLine := fmt.Sprintf("Start Time: %q, Tick number: %d", startTime, count)
				log.Output(1, logLine)
			// can it live for a whole hour without dying?
				if (count > 240) {
					close(quit)
				}
			// do stuff
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	c.String(http.StatusOK, "push notification sent")
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