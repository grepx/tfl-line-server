package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/russross/blackfriday"
	_ "github.com/lib/pq"
	"encoding/json"
)

var (
	repeat int
	db     *sql.DB
)

func repeatFunc(c *gin.Context) {
	var buffer bytes.Buffer
	for i := 0; i < repeat; i++ {
		buffer.WriteString("Hello from Go!")
	}
	c.String(http.StatusOK, buffer.String())
}

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

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	var err error
	tStr := os.Getenv("REPEAT")
	repeat, err = strconv.Atoi(tStr)
	if err != nil {
		log.Print("Error converting $REPEAT to an int: %q - Using default", err)
		repeat = 5
	}

	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL") + " sslmode=disable")
	if err != nil {
		log.Fatalf("Error opening database: %q", err)
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.LoadHTMLGlob("templates/*.tmpl.html")
	router.Static("/static", "static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})

	router.GET("/mark", func(c *gin.Context) {
		c.String(http.StatusOK, string(blackfriday.MarkdownBasic([]byte("**hi!**"))))
	})

	router.GET("/repeat", repeatFunc)
	router.GET("/db", dbFunc)

	router.GET("/showForecast", showForecast)
	router.GET("/fetchForecast", fetchForecast)

	router.Run(":" + port)
}

func showForecast(c *gin.Context) {
	createForecastTable(c)
	rows, err := db.Query("SELECT timestamp, description FROM forecast")
	if err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("Error reading ticks: %q", err))
		return
	}

	defer rows.Close()
	for rows.Next() {
		var timestamp time.Time
		var description string
		if err := rows.Scan(&timestamp, &description); err != nil {
			c.String(http.StatusInternalServerError,
				fmt.Sprintf("Error scanning timestamps: %q", err))
			return
		}
		c.String(http.StatusOK, fmt.Sprintf("Read from DB: %s, %s\n", timestamp.String(), description))
	}
}

func fetchForecast(c *gin.Context) {
	createForecastTable(c)

	if _, err := db.Exec("INSERT INTO forecast VALUES (now(), $1)", "greg"); err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("Error incrementing tick: %q", err))
		return
	}


	record := doNetworkCall()
	c.String(http.StatusOK, fmt.Sprintf("Read from network: %s\n", record.Cod))

	return
}

func createForecastTable(c *gin.Context) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS forecast (timestamp timestamp, description text)");
	if err != nil {
		c.String(http.StatusInternalServerError,
			fmt.Sprintf("Error creating database table: %q", err))
		return
	}
}

type ForecastResponse struct {
	Cod string `json:"cod"`
}

func doNetworkCall() ForecastResponse {
	// Fill the record with the data from the JSON
	var forecastResponse ForecastResponse


	url := "http://samples.openweathermap.org/data/2.5/forecast?q=London,us&appid=b1b15e88fa797225412429c1c50c122a1"

	// Build the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal("NewRequest: ", err)
		return forecastResponse
	}

	// For control over HTTP client headers,
	// redirect policy, and other settings,
	// create a Client
	// A Client is an HTTP client
	client := &http.Client{}

	// Send the request via a client
	// Do sends an HTTP request and
	// returns an HTTP response
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Do: ", err)
		return forecastResponse
	}

	// Callers should close resp.Body
	// when done reading from it
	// Defer the closing of the body
	defer resp.Body.Close()


	// Use json.Decode for reading streams of JSON data
	if err := json.NewDecoder(resp.Body).Decode(&forecastResponse); err != nil {
		log.Println(err)
	}

	return forecastResponse
}