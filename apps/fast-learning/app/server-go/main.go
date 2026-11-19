package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"time"

	learning "claude-code-go/apps/fast-learning"
	"claude-code-go/pkg/appagent"
)

func main() {
	socket, data := os.Getenv("CLAUDE_APP_SOCKET"), os.Getenv("CLAUDE_APP_DATA_DIR")
	if socket == "" || data == "" {
		log.Fatal("launch Fast Learning through the Agent App runtime")
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatal(err)
	}
	agent := appagent.NewClient(os.Getenv("CLAUDE_APP_AGENT_SOCKET"))
	server := &http.Server{Handler: learning.New(data, agent), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.Serve(listener))
}
