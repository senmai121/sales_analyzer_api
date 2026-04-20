package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"sales_analyzer_api/internal/llm"
	"sales_analyzer_api/internal/db"
	"sales_analyzer_api/internal/handlers"
)

func main() {
	// Load .env if it exists (optional — env vars may already be set)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	ctx := context.Background()

	// Connect to database
	pool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Println("Database connected")

	// Create Claude client
	claudeClient, err := llm.New()
	if err != nil {
		log.Fatalf("Failed to create Claude client: %v", err)
	}
	log.Println("Claude client initialized")

	// Create handlers
	summaryHandler := handlers.NewSummaryHandler(pool, claudeClient)
	searchHandler := handlers.NewSearchHandler(pool, claudeClient)
	rankingHandler := handlers.NewRankingHandler(pool, claudeClient)
	insightsHandler := handlers.NewInsightsHandler(pool, claudeClient)
	similarHandler := handlers.NewSimilarHandler(pool)
	categoriesHandler := handlers.NewCategoriesHandler(pool)

	// Set up router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/insights", insightsHandler.ServeHTTP)
		r.Get("/categories", categoriesHandler.ServeHTTP)

		r.Route("/products", func(r chi.Router) {
			r.Get("/search", searchHandler.ServeHTTP)
			r.Get("/ranking", rankingHandler.ServeHTTP)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/summary", summaryHandler.ServeHTTP)
				r.Get("/similar", similarHandler.ServeHTTP)
			})
		})
	})

	// Determine port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
