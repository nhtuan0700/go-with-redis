package main

import (
	"log"
	"math/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nhtuan0700/go-with-redis/cache"
)

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"

	result := make([]byte, n)
	for i := range result {
		result[i] = letters[rand.Intn(len(letters))]
	}

	return string(result)
}

type SessionRequest struct {
	Username string `json:"user_name"`
}

// tạo session cho mỗi người đăng nhập, dùng redis để lưu session id, user name ấy
func makeSession(sessionClient cache.SessionClient) func(c *gin.Context) {
	return func(c *gin.Context) {
		var req = new(SessionRequest)

		if err := c.BindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, "invalid params")
			return
		}
		if req.Username == "" {
			c.JSON(http.StatusBadRequest, "username is required")
			return
		}

		sessionID := randomString(5)
		err := sessionClient.MakeSession(c, sessionID, req.Username)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"session_id": sessionID})
	}
}

type PingRequest struct {
	SessionID string `json:"session_id"`
}

// chỉ cho phép 1 người được gọi tại một thời điểm ( với sleep ở bên trong api đó trong 5s)
// rate limit mỗi người chỉ được gọi API /ping 2 lần trong 60s
// đếm số lượng lần 1 người gọi api /ping
func ping(sessionClient cache.SessionClient) func(c *gin.Context) {
	return func(c *gin.Context) {
		req := new(PingRequest)

		if err := c.BindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		if req.SessionID == "" {
			c.JSON(http.StatusBadRequest, "session is required")
			return
		}

		// Check if user is allowed to ping
		isAllow, err := sessionClient.IsAllowPing(c, req.SessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		if !isAllow {
			c.JSON(http.StatusBadRequest, gin.H{"message": "You just pinged. Please try again after 5s"})
			return
		}

		// rate limit mỗi người chỉ được gọi API /ping 2 lần trong 60s
		err = sessionClient.MakeRateLimitPing(c, req.SessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		// đếm số lượng lần 1 người gọi api /ping
		// bao gồm: chỉ cho phép 1 người được gọi tại một thời điểm ( với sleep ở bên trong api đó trong 5s)
		count, err := sessionClient.MakePing(c, req.SessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"message":    "pong",
			"ping_count": count,
		})
	}
}

// trả về top 10 người gọi API /ping nhiều nhất
func getTopHighestPing(sessionClient cache.SessionClient) func(c *gin.Context) {
	return func(c *gin.Context) {
		result, err := sessionClient.GetTopHighestPing(c, 10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// lưu xấp sỉ số người gọi api /ping , và trả về trong api /count
func getUserPingedCount(sessionClient cache.SessionClient) func(c *gin.Context) {
	return func(c *gin.Context) {
		count, err := sessionClient.GetUserPingedCount(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"count": count})
	}
}

func main() {
	r := gin.Default()
	redisClient := cache.NewRedisClient()
	sessionClient := cache.NewSessionClient(redisClient)

	r.POST("/session", makeSession(sessionClient))
	r.POST("/ping", ping(sessionClient))
	r.GET("/ping/top", getTopHighestPing(sessionClient))
	r.GET("/ping/count", getUserPingedCount(sessionClient))

	addr := ":8080"
	server := http.Server{
		Addr:    addr,
		Handler: r,
	}
	log.Println("Starting server: ", addr)
	err := server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
