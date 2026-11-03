package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/redis/go-redis/v9"
)

type evaluator struct {
	redis       *redis.Client
	httpClient  *http.Client
	flagURL     string
	targetURL   string
	eventDriver string
	queueURL    string
	queueKey    string
	sqs         *sqs.Client
	workMS      int
}

type flag struct {
	Name      string `json:"name"`
	IsEnabled bool   `json:"is_enabled"`
}

type targetResult struct {
	Matches bool `json:"matches"`
}

type evaluationEvent struct {
	EventID    string         `json:"event_id"`
	FlagName   string         `json:"flag_name"`
	SubjectID  string         `json:"subject_id"`
	Enabled    bool           `json:"enabled"`
	Cached     bool           `json:"cached"`
	Attributes map[string]any `json:"attributes"`
	EvaluatedAt time.Time     `json:"evaluated_at"`
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func (e *evaluator) health(w http.ResponseWriter, r *http.Request) {
	if err := e.redis.Ping(r.Context()).Err(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "error": "redis unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func doJSON(client *http.Client, method, endpoint string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (e *evaluator) evaluate(w http.ResponseWriter, r *http.Request) {
	flagName := strings.TrimPrefix(r.URL.Path, "/evaluation/")
	if flagName == "" || flagName == r.URL.Path {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "flag name is required"})
		return
	}
	subjectID := r.URL.Query().Get("subject_id")
	if subjectID == "" {
		subjectID = "anonymous"
	}
	attributes := map[string]any{}
	if encoded := r.URL.Query().Get("attributes"); encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &attributes); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attributes must be valid JSON"})
			return
		}
	}

	attributeBytes, _ := json.Marshal(attributes)
	keyDigest := sha256.Sum256(attributeBytes)
	cacheKey := fmt.Sprintf("evaluation:%s:%s:%s", flagName, subjectID, hex.EncodeToString(keyDigest[:8]))
	ctx := r.Context()
	cached, err := e.redis.Get(ctx, cacheKey).Result()
	fromCache := err == nil
	if fromCache {
		var result bool
		_ = json.Unmarshal([]byte(cached), &result)
		_ = e.publish(ctx, evaluationEvent{EventID: fmt.Sprintf("%d", time.Now().UnixNano()), FlagName: flagName, SubjectID: subjectID, Enabled: result, Cached: true, Attributes: attributes, EvaluatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusOK, map[string]any{"flag_name": flagName, "enabled": result, "cached": true})
		return
	}

	var flagData flag
	if err := doJSON(e.httpClient, http.MethodGet, strings.TrimRight(e.flagURL, "/")+"/flags/"+url.PathEscape(flagName), nil, &flagData); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "flag-service unavailable"})
		return
	}
	result := flagData.IsEnabled
	if result {
		var target targetResult
		err := doJSON(e.httpClient, http.MethodPost, strings.TrimRight(e.targetURL, "/")+"/targeting/match/"+url.PathEscape(flagName), map[string]any{"subject_id": subjectID, "attributes": attributes}, &target)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "targeting-service unavailable"})
			return
		}
		result = target.Matches
	}
	if e.workMS > 0 {
		deadline := time.Now().Add(time.Duration(e.workMS) * time.Millisecond)
		seed := []byte(flagName)
		for time.Now().Before(deadline) {
			digest := sha256.Sum256(seed)
			seed = digest[:]
		}
	}
	_ = e.redis.Set(ctx, cacheKey, strconv.FormatBool(result), 30*time.Second).Err()
	_ = e.publish(ctx, evaluationEvent{EventID: fmt.Sprintf("%d", time.Now().UnixNano()), FlagName: flagName, SubjectID: subjectID, Enabled: result, Cached: false, Attributes: attributes, EvaluatedAt: time.Now().UTC()})
	writeJSON(w, http.StatusOK, map[string]any{"flag_name": flagName, "enabled": result, "cached": false})
}

func (e *evaluator) publish(ctx context.Context, event evaluationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if e.eventDriver == "redis" {
		return e.redis.RPush(ctx, e.queueKey, string(payload)).Err()
	}
	if e.sqs == nil || e.queueURL == "" {
		return fmt.Errorf("SQS is not configured")
	}
	_, err = e.sqs.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(e.queueURL), MessageBody: aws.String(string(payload))})
	return err
}

func main() {
	ctx := context.Background()
	redisClient := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "redis:6379"), Password: os.Getenv("REDIS_PASSWORD")})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}
	e := &evaluator{
		redis: redisClient,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		flagURL: env("FLAG_SERVICE_URL", "http://flag-service:8080"),
		targetURL: env("TARGETING_SERVICE_URL", "http://targeting-service:8080"),
		eventDriver: env("EVENT_BUS_DRIVER", "redis"),
		queueURL: os.Getenv("SQS_QUEUE_URL"),
		queueKey: env("REDIS_QUEUE_KEY", "analytics-events"),
	}
	if work, err := strconv.Atoi(env("EVALUATION_WORK_MS", "0")); err == nil {
		e.workMS = work
	}
	if e.eventDriver == "sqs" {
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(env("AWS_REGION", "us-east-1")))
		if err != nil {
			log.Fatal(err)
		}
		e.sqs = sqs.NewFromConfig(cfg)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", e.health)
	mux.HandleFunc("/evaluation/", e.evaluate)
	port := env("PORT", "8080")
	log.Printf("evaluation-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
