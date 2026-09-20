package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"gdcpay/internal/config"
	"gdcpay/internal/handler"
	"gdcpay/internal/middleware"
	"gdcpay/internal/notification"
	"gdcpay/internal/pkg/jwtutil"
	"gdcpay/internal/repository/postgres"
	"gdcpay/internal/service"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	db, err := postgres.NewDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	tokenizer := jwtutil.NewTokenizer(cfg.JWTSecret, cfg.JWTExpiresIn)

	userRepo := postgres.NewUserRepository(db)
	teamRepo := postgres.NewTeamRepository(db)
	taskRepo := postgres.NewTaskRepository(db)
	taskLogRepo := postgres.NewTaskLogRepository(db)
	idempotencyStore := postgres.NewIdempotencyStore(db)
	txManager := postgres.NewTxManager(db)
	notifier := notification.NewLogNotifier(logger)

	authService := service.NewAuthService(userRepo, teamRepo, tokenizer, cfg.JWTExpiresIn)
	authHandler := handler.NewAuthHandler(authService)

	taskService := service.NewTaskService(service.TaskServiceDeps{
		Tasks:          taskRepo,
		Users:          userRepo,
		TaskLogs:       taskLogRepo,
		Notifier:       notifier,
		TxManager:      txManager,
		Idempotency:    idempotencyStore,
		IdempotencyTTL: cfg.IdempotencyTTL,
	})
	taskHandler := handler.NewTaskHandler(taskService)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.Recovery(logger), middleware.RequestLogger(logger))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authGroup := router.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	protected := router.Group("/")
	protected.Use(middleware.Auth(tokenizer))
	{
		protected.POST("/tasks", taskHandler.Create)
		protected.GET("/tasks", taskHandler.List)
		protected.GET("/tasks/:id", taskHandler.Get)
		protected.PUT("/tasks/:id", taskHandler.Update)
		protected.DELETE("/tasks/:id", taskHandler.Delete)
		protected.POST("/tasks/:id/assign", taskHandler.Assign)
	}

	logger.Info("starting server", "port", cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
