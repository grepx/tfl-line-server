package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"encoding/json"
	"io/ioutil"
	"github.com/NaySoftware/go-fcm"
	"time"
	"strconv"
)

var (
	apiKey string
	firebaseKey string
	pollingIntervalSeconds int
	quitChannel chan struct{}
	previousLines map[string]Line
)

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	// load the api key from config
	// this is used to protect the endpoints from unknown callers
	apiKey = os.Getenv("API_KEY")

	// load firebase key
	firebaseKey = os.Getenv("FIREBASE_KEY")

	var err error
	pollingIntervalSeconds, err = strconv.Atoi(os.Getenv("POLLING_INTERVAL_SECONDS"))
	if (err != nil) {
		log.Fatal("$POLLING_INTERVAL_SECONDS must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/startService", startService)
	router.GET("/doTestPushNotification", doTestPushNotification)

	router.Run(":" + port)
}

func checkApiKeyIsValid(c *gin.Context) bool {
	apiKeyParam := c.Query("apiKey")
	if (apiKey != apiKeyParam) {
		log.Output(1, "API KEY mistmatch: " + apiKeyParam)
		return false
	}
	return true
}

func doTestPushNotification(c *gin.Context) {
	if (!checkApiKeyIsValid(c)) {
		return
	}
	lineId := c.Query("lineId")
	lineStatus, _ := strconv.Atoi(c.Query("lineStatus"))
	lineStatusDescription := c.Query("lineStatusDescription")
	reason := c.Query("reason")
	sendStatusNotification(lineId, lineStatus, lineStatusDescription, reason)
	c.String(http.StatusOK, "sent test notification")
}

func startService(c *gin.Context) {
	if (!checkApiKeyIsValid(c)) {
		return
	}
	// switch off any previous service
	if (quitChannel != nil) {
		close(quitChannel)
	}
	// spawn a timer that keeps calling on a go channel every 10 seconds
	// this will run until the next startService call shuts it down
	log.Output(1, fmt.Sprintf("Starting a service that runs every %d seconds", pollingIntervalSeconds))
	ticker := time.NewTicker(time.Duration(pollingIntervalSeconds) * time.Second)
	quitChannel = make(chan struct{})
	secondsCount := 0

	go func() {
		for {
			select {
			case <-ticker.C:
				pollLineStatus()
			// in the unlikely case that a service doesn't get shut down correctly,
			// don't let it run forever, just for 1000 seconds, 16 minutes
				secondsCount += 10
				if (secondsCount > 1000) {
					log.Output(1, "WARNING: something happened and the timer kept running after it should have reset.")
					close(quitChannel)
				}
			case <-quitChannel:
				ticker.Stop()
				return
			}
		}
	}()
	// return right away with success
	c.String(http.StatusOK, "service started")
}

func pollLineStatus() {
	log.Output(1, "Polling line status")
	currentLinesJson, err := fetchLinesJson()
	if (err != nil) {
		return
	}

	currentLines := decodeLines(currentLinesJson)

	if (previousLines != nil) {
		checkLineStatuses(currentLines)
	} else {
		log.Output(1, "WARNING: there was no previous line state, you must be starting up for the first time?")
	}

	previousLines = currentLines;
}

func checkLineStatuses(currentLines map[string]Line) {
	for lineId, line := range currentLines {
		// just check the first line status (seems like the list isn't really used?)
		lineStatus := line.LineStatuses[0]
		previousLine := previousLines[lineId]
		previousLineStatus := previousLine.LineStatuses[0]
		if (lineStatus.StatusSeverity != previousLineStatus.StatusSeverity) {
			// send push notification with the new status
			sendStatusNotification(line.Id, lineStatus.StatusSeverity,
				lineStatus.StatusSeverityDescription, lineStatus.Reason)
		}
		log.Output(1, "checked line " + lineId)
	}
}

func decodeLines(linesJson string) map[string]Line {
	byt := []byte(linesJson)
	lines := make([]Line, 0)

	if err := json.Unmarshal(byt, &lines); err != nil {
		panic(err)
	}

	lineMap := make(map[string]Line)
	for _, line := range lines {
		lineMap[line.Id] = line
	}
	return lineMap
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

type Line struct {
	Id           string `json:"id"`
	LineStatuses []LineStatus `json:"lineStatuses"`
}

type LineStatus struct {
	Id                        int `json:"id"`
	StatusSeverity            int `json:"statusSeverity"`
	StatusSeverityDescription string `json:"statusSeverityDescription"`
	Reason                    string `json:reason`
}

func sendStatusNotification(lineId string, lineStatus int, lineStatusDescription string, reason string) {
	data := map[string]string{
		"lineId": lineId,
		"lineStatus": strconv.Itoa(lineStatus),
		"lineStatusDescription": lineStatusDescription,
		"reason": reason,
	}

	dataJson, _ := json.Marshal(data)
	log.Output(1, "Sending push notification: " + string(dataJson))

	c := fcm.NewFcmClient(firebaseKey)
	c.NewFcmMsgTo("/topics/" + lineId, data)

	status, err := c.Send()

	if err == nil {
		status.PrintResults()
	} else {
		fmt.Println(err)
	}
}