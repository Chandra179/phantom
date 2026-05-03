package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"phantom/pkg/ingestion"
	"phantom/pkg/shared"
)

func main() {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Pipeline wiring — uses in-memory store for now.
	// Replace store with ParquetStore for persistence.
	store := ingestion.NewMemStore()
	apiKey := os.Getenv("STOOQ_APIKEY")

	pipe := &ingestion.Pipeline{
		Fetcher: &ingestion.StooqFetcher{APIKey: apiKey},
		Deduper: &ingestion.MemDeduper{},
		Store:   store,
		Limiter: rate.NewLimiter(rate.Every(time.Second), 5),
	}

	r.POST("/ingest", func(c *gin.Context) {
		var req struct {
			Ticker string `json:"ticker" binding:"required"`
			From   string `json:"from" binding:"required"`
			To     string `json:"to" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		from, err := time.Parse("2006-01-02", req.From)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date: " + err.Error()})
			return
		}
		to, err := time.Parse("2006-01-02", req.To)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date: " + err.Error()})
			return
		}

		tr := shared.TimeRange{From: from, To: to}
		if err := pipe.Run(context.Background(), shared.AssetID(req.Ticker), tr); err != nil {
			log.Printf("pipeline error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "ticker": req.Ticker})
	})

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
