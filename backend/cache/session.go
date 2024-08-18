package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const userNameSetKey = "user_name_set"
const leaderBoardPingSetKey = "leader_board_ping_set"
const userPingedCountKey = "user_ping_count"

// getSessionIDKey return session_id:xxx
func getSessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

func getRateLimitIn5s(id string) string {
	return fmt.Sprintf("session_rate_limit_5s:%s", id)
}

func getRateLimitPingCountKey(id string) string {
	return fmt.Sprintf("session_rate_limit_ping:%s", id)
}

// TODO: it need to be placed another place
type Session struct {
	UserName  string `json:"user_name"`
	PingCount uint64 `json:"ping_count"`
}

type Score struct {
	UserName  string
	PingCount uint64
}

type SessionClient interface {
	// MakeSession create session data
	MakeSession(ctx context.Context, sessionID string, userName string) error

	// GetSession get session data
	GetSession(ctx context.Context, userName string) (Session, error)
	// MakePing counting ping for each session
	MakePing(ctx context.Context, sessionID string) (uint64, error)
	// MakeRateLimitPing Limit each session to ping only 2 times in 60s
	MakeRateLimitPing(ctx context.Context, sessionID string) error
	// IsAllowPing Check if user is allowed to ping
	IsAllowPing(ctx context.Context, sessionID string) (bool, error)

	// GetTopHighestPing get top highest ping
	GetTopHighestPing(ctx context.Context, limit uint64) ([]Score, error)

	// GetUserPingedCount get total ping count
	GetUserPingedCount(ctx context.Context) (uint64, error)
}

type sessionClient struct {
	client Client
}

// NewSessionClient
func NewSessionClient(client Client) SessionClient {
	return &sessionClient{
		client: client,
	}
}

// setSession set session data into cache
func (c *sessionClient) setSession(ctx context.Context, sessionID string, session Session) error {
	sessionString, err := json.Marshal(session)
	if err != nil {
		return errors.New("cannot marshal json session")
	}

	err = c.client.Set(ctx, getSessionKey(sessionID), sessionString, 0)
	if err != nil {
		return err
	}
	return nil
}

// GetSession get session data
func (c *sessionClient) GetSession(ctx context.Context, sessionID string) (Session, error) {
	cacheEntry, err := c.client.Get(ctx, getSessionKey(sessionID))
	if err != nil {
		return Session{}, err
	}

	sessionData, ok := cacheEntry.(string)
	if !ok {
		log.Println("[Error]: cache entry is not type string")
		return Session{}, nil
	}

	var session Session
	if err = json.Unmarshal([]byte(sessionData), &session); err != nil {
		return Session{}, errors.New("can not unmarshal json session")
	}

	return session, nil
}

// MakeSession create session data
func (c *sessionClient) MakeSession(ctx context.Context, sessionID string, userName string) error {
	isExist, err := c.client.IsDataInSet(ctx, userNameSetKey, userName)
	if err != nil {
		return err
	}

	if isExist {
		return errors.New("username is taken by another one")
	}

	// sorted set leader board
	err = c.client.AddToSet(ctx, userNameSetKey, userName)
	if err != nil {
		return err
	}

	err = c.setSession(ctx, sessionID, Session{
		UserName:  userName,
		PingCount: 0,
	})

	if err != nil {
		return err
	}

	return nil
}

// MakePing counting ping for each session
func (c *sessionClient) MakePing(ctx context.Context, sessionID string) (uint64, error) {
	session, err := c.GetSession(ctx, sessionID)
	if err != nil {
		return 0, err
	}

	session.PingCount += 1
	sessionString, err := json.Marshal(session)
	if err != nil {
		return 0, errors.New("cannot marshal json session")
	}
	err = c.client.Set(ctx, getSessionKey(sessionID), sessionString, 0)
	if err != nil {
		return 0, err
	}
	// rate limit for ping in 5s
	err = c.client.Set(ctx, getRateLimitIn5s(sessionID), 1, 5*time.Second)
	if err != nil {
		return 0, err
	}

	err = c.setSession(ctx, sessionID, session)
	if err != nil {
		return 0, err
	}

	// add to sorted set
	err = c.client.AddToSortedSet(ctx, leaderBoardPingSetKey, redis.Z{Member: session.UserName, Score: float64(session.PingCount)})
	if err != nil {
		return 0, err
	}

	// add to hyperloglog
	err = c.client.AddHyperLogLog(ctx, userPingedCountKey, session.UserName)
	if err != nil {
		return 0, err
	}

	return session.PingCount, nil
}

// MakeRateLimitPing Limit each session to ping only 2 times in 60s
func (c *sessionClient) MakeRateLimitPing(ctx context.Context, sessionID string) error {
	rateLimitPingCountKey := getRateLimitPingCountKey(sessionID)

	cacheEntry, err := c.client.Get(ctx, rateLimitPingCountKey)
	if err != nil {
		if !errors.Is(err, ErrCacheMiss) {
			return err
		}
	}

	cachedData, ok := cacheEntry.(string)
	if !ok {
		// init
		cachedData = "1"
	}

	log.Println("cachedData: ", cachedData)

	count, err := strconv.Atoi(cachedData)
	if err != nil {
		return errors.New("can not parse rateLimit to number")
	}

	if count == 2 {
		return errors.New("exceed ping limit. Please try after 60s")
	}

	// count + 1 in cache
	err = c.client.Incr(ctx, rateLimitPingCountKey)
	if err != nil {
		return err
	}

	// make sure rateLimitPingCountKey is existed in cache and is init
	if count == 1 {
		err = c.client.Expire(ctx, rateLimitPingCountKey, 60*time.Second)
		if err != nil {
			return err
		}
	}

	return nil
}

// IsAllowPing Check if user is allowed to ping
func (c *sessionClient) IsAllowPing(ctx context.Context, sessionID string) (bool, error) {
	_, err := c.client.Get(ctx, getRateLimitIn5s(sessionID))
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return true, nil
		}

		return false, err
	}

	return false, nil
}

// GetTopHighestPing get top highest ping
func (c *sessionClient) GetTopHighestPing(ctx context.Context, limit uint64) ([]Score, error) {
	cachedData, err := c.client.GetSortedSet(ctx, leaderBoardPingSetKey, OrderDesc)
	if err != nil {
		return []Score{}, err
	}

	if int(limit) < len(cachedData) {
		cachedData = cachedData[0:limit]
	}

	var result = make([]Score, len(cachedData))
	for i, item := range cachedData {
		userName, ok := item.Member.(string)
		if !ok {
			return []Score{}, errors.New("can not parse userName to string")
		}

		score := uint64(item.Score)

		result[i] = Score{
			UserName:  userName,
			PingCount: score,
		}
	}

	return result, nil
}

// GetUserPingedCount get total ping count
func (c *sessionClient) GetUserPingedCount(ctx context.Context) (uint64, error) {
	count, err := c.client.GetHyperLogLogCount(ctx, userPingedCountKey)
	if err != nil {
		return 0, err
	}
	return count, nil
}
