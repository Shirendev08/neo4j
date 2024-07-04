package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

var driver neo4j.Driver

type Movie struct {
	ID       int64  `json:"id"`
	Released int64  `json:"released"`
	Tagline  string `json:"tagline"`
	Title    string `json:"title"`
}

func main() {
	var err error
	driver, err = neo4j.NewDriver("neo4j://localhost:7687", neo4j.BasicAuth("neo4j", "12345678", ""))
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close()

	router := gin.Default()
	router.GET("/movies", getMovies)
	router.GET("/movies/:id", getMovieByID)
	router.GET("/persons", getPeople)
	router.POST("/movies", createMovie)
	router.POST("/deleteMovie/:id", deleteMovieByID)
	router.POST("/moviesUpdate", updateMovie)
	log.Fatal(router.Run(":8080"))
}

func getMovies(c *gin.Context) {
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	result, err := session.Run("MATCH (n:Movie) RETURN id(n), n.released, n.tagline, n.title", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var movies []Movie
	for result.Next() {
		record := result.Record()
		id, ok := record.GetByIndex(0).(int64)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cast id to int64"})
			return
		}
		released, ok := record.GetByIndex(1).(int64)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cast released to int"})
			return
		}
		tagline, ok := record.GetByIndex(2).(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cast tagline to string"})
			return
		}
		title, ok := record.GetByIndex(3).(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cast title to string"})
			return
		}
		movie := Movie{
			ID:       id,
			Released: released,
			Tagline:  tagline,
			Title:    title,
		}
		movies = append(movies, movie)
	}
	if err = result.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"movies": movies})
}

func getPeople(c *gin.Context) {
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()
	result, err := session.Run("Match (n:Person) return n.name", nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var persons []string
	for result.Next() {
		persons = append(persons, result.Record().GetByIndex(0).(string))
	}
	if err = result.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"humuus": persons})
}

func createMovie(c *gin.Context) {
	var movie struct {
		Title    string `json:"title"`
		Tagline  string `json:"tagline"`
		Released int    `json:"released"`
	}

	if err := c.ShouldBindJSON(&movie); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	_, err := session.Run(
		"CREATE (n:Movie {title: $title, tagline: $tagline, released: $released})",
		map[string]interface{}{
			"title":    movie.Title,
			"tagline":  movie.Tagline,
			"released": movie.Released,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created"})
}

func updateMovie(c *gin.Context) {
	var update struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Tagline  string `json:"tagline"`
		Released int64  `json:"released"`
	}
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	// Check if the movie exists
	result, err := session.Run(
		"MATCH (n:Movie) WHERE ID(n) = $id RETURN n.title, n.tagline, n.released",
		map[string]interface{}{"id": update.ID},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !result.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Movie not found"})
		return
	}

	// Retrieve old values
	oldTitle := result.Record().GetByIndex(0).(string)
	oldTagline := result.Record().GetByIndex(1).(string)
	oldReleased := result.Record().GetByIndex(2).(int64)

	// Update the movie properties
	query := "MATCH (n:Movie) WHERE ID(n) = $id SET "
	params := map[string]interface{}{"id": update.ID}

	if update.Title != "" {
		query += "n.title = $newTitle, "
		params["newTitle"] = update.Title
	}
	if update.Tagline != "" {
		query += "n.tagline = $newTagline, "
		params["newTagline"] = update.Tagline
	}
	if update.Released != 0 {
		query += "n.released = $newReleased, "
		params["newReleased"] = update.Released
	}

	// Remove the trailing comma and space
	query = strings.TrimSuffix(query, ", ")

	_, err = session.Run(query, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Prepare response
	message := fmt.Sprintf("Movie details updated for ID %d:", update.ID)
	if update.Title != "" {
		message += fmt.Sprintf("\n- Title changed from \"%s\" to \"%s\"", oldTitle, update.Title)
	}
	if update.Tagline != "" {
		message += fmt.Sprintf("\n- Tagline changed from \"%s\" to \"%s\"", oldTagline, update.Tagline)
	}
	if update.Released != 0 {
		message += fmt.Sprintf("\n- Released year changed from \"%d\" to \"%d\"", oldReleased, update.Released)
	}

	c.JSON(http.StatusOK, gin.H{"message": message})
}

func getMovieByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid movie ID"})
		return
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	result, err := session.Run(
		"MATCH (n:Movie) WHERE id(n) = $id RETURN id(n), n.released, n.tagline, n.title",
		map[string]interface{}{"id": id},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.Next() {
		record := result.Record()
		movie := Movie{
			ID:       record.GetByIndex(0).(int64),
			Released: int64(record.GetByIndex(1).(int64)), // assuming released is stored as int64
			Tagline:  record.GetByIndex(2).(string),
			Title:    record.GetByIndex(3).(string),
		}
		c.JSON(http.StatusOK, gin.H{"movie": movie})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "Movie not found"})
	}

	if err = result.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
}

func deleteMovieByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid movie ID"})
		return
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	result, err := session.Run(
		"MATCH (n:Movie) WHERE id(n) = $id DELETE n",
		map[string]interface{}{"id": id},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if any nodes were actually deleted
	summary, err := result.Consume()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if summary.Counters().NodesDeleted() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Movie not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
