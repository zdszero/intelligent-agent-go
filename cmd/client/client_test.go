package main

import (
	"smart-agent/config"
	"testing"
)

func TestPing(t *testing.T) {
	pingServer("127.0.0.1", config.PingPort)
}
