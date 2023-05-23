package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"smart-agent/config"
	"smart-agent/util"
	"sync"

	"github.com/go-redis/redis/v8"
)

func main() {
	// Create redis client
	redisCli := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("localhost:%d", config.RedisPort), // Redis server address
		Password: "",                                            // Redis server password
		DB:       0,                                             // Redis database number
	})
	defer redisCli.Close()

	// Ping the Redis server to check the connection
	pong, err := redisCli.Ping(context.Background()).Result()
	if err != nil {
		log.Println("Failed to connect to Redis:", err)
	}
	log.Println("Connected to Redis:", pong)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		listener := util.CreateMptcpListener(config.ClientServePort)
		defer listener.Close()
		// Accept and handle client connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Failed to accept client connection:", err)
				continue
			}

			go handleClient(conn, redisCli)
		}
	}()

	go func() {
		defer wg.Done()
		listener := util.CreateMptcpListener(config.DataTransferPort)
		defer listener.Close()
		// Accept and handle client connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Failed to accept transfer connection:", err)
				continue
			}

			go handleTransfer(conn, redisCli)
		}
	}()
	wg.Wait()
}

func handleClient(conn net.Conn, redisCli *redis.Client) {
	defer conn.Close()

	var clientId string
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.ClientId {
			log.Printf("Client Id Enter: %v", data)
			clientId = data
		} else if cmd == config.ClientData {
			fmt.Println("rpush", clientId, data)
			err := redisCli.RPush(context.Background(), clientId, data).Err()
			if err != nil {
				log.Println("Failed to push values to Redis list:", err)
				return
			}
		} else if cmd == config.ClientExit {
			log.Printf("Client Id Exit: %v", clientId)
			break
		} else if cmd == config.FetchClientData {

		} else if cmd == config.FetchOldData {
		}
	}
}

func fetchData(clusterIp string) {

}

func handleTransfer(conn net.Conn, redisCli *redis.Client) {
	cmd, clientId := util.RecvNetMessage(conn)
	if cmd != config.ClientId {
		log.Println("Error: expected client id in the beginning of transfer")
	}
	result, err := redisCli.LRange(context.Background(), clientId, 0, -1).Result()
	if err != nil {
		log.Println("Error:", err)
		return
	}
	for _, element := range result {
		util.SendNetMessage(conn, config.TransferData, element)
	}
	util.SendNetMessage(conn, config.TransferEnd, "")
}
