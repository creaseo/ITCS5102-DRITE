package drite

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAuthToken = "drite-demo-token"

type Config struct {
	AuthToken     string
	CPUThreshold  float64
	AlertCooldown time.Duration
	PingInterval  time.Duration
	PongWait      time.Duration
	WriteWait     time.Duration
	ReadLimitByte int64
}

func defaultConfig(c Config) Config {
	if c.AuthToken == "" {
		c.AuthToken = defaultAuthToken
	}
	if c.CPUThreshold == 0 {
		c.CPUThreshold = 90
	}
	if c.CPUThreshold < 0 || c.CPUThreshold > 100 || math.IsNaN(c.CPUThreshold) || math.IsInf(c.CPUThreshold, 0) {
		panic("CPU threshold must be between 0 and 100")
	}
	if c.AlertCooldown <= 0 {
		c.AlertCooldown = 5 * time.Second
	}
	if c.PingInterval <= 0 {
		c.PingInterval = 20 * time.Second
	}
	if c.PongWait <= 0 {
		c.PongWait = 60 * time.Second
	}
	if c.WriteWait <= 0 {
		c.WriteWait = 10 * time.Second
	}
	if c.ReadLimitByte <= 0 {
		c.ReadLimitByte = 4096
	}
	if c.PingInterval >= c.PongWait {
		panic("ping interval must be less than pong wait")
	}
	return c
}

// LoadConfig preserves the environment variables from the original README.
func LoadConfig() (Config, string, error) {
	c := Config{AuthToken: os.Getenv("DRITE_AUTH_TOKEN"), CPUThreshold: 90, AlertCooldown: 5 * time.Second}
	if raw, ok := os.LookupEnv("DRITE_CPU_THRESHOLD"); ok {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 100 {
			return c, "", fmt.Errorf("DRITE_CPU_THRESHOLD must be in (0,100]")
		}
		c.CPUThreshold = v
	}
	if raw, ok := os.LookupEnv("DRITE_ALERT_COOLDOWN"); ok {
		if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
			c.AlertCooldown = d
		} else if sec, err := strconv.ParseFloat(raw, 64); err == nil && sec >= 0 {
			c.AlertCooldown = time.Duration(sec * float64(time.Second))
		} else {
			return c, "", fmt.Errorf("DRITE_ALERT_COOLDOWN must be non-negative duration or seconds")
		}
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, "", fmt.Errorf("PORT must be an integer in [1,65535]")
	}
	return defaultConfig(c), ":" + port, nil
}
