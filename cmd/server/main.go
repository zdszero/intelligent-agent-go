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

type AgentServer struct {
	redisCli *redis.Client
}

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

	ser := AgentServer{
		redisCli: redisCli,
	}

	var wg sync.WaitGroup
	wg.Add(3)
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

			go ser.handleClient(conn)
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

			go ser.handleTransfer(conn)
		}
	}()

	go func() {
		defer wg.Done()
		serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", config.PingPort))
		if err != nil {
			fmt.Println("Error resolving server address:", err)
			return
		}

		conn, err := net.ListenUDP("udp", serverAddr)
		if err != nil {
			fmt.Println("Error listening:", err)
			return
		}
		defer conn.Close()

		buffer := make([]byte, 1024)

		for {
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Println("Error reading message:", err)
				continue
			}

			fmt.Printf("Received ping from %s: %s\n", addr.String(), string(buffer[:n]))

			_, err = conn.WriteToUDP([]byte("pong"), addr)
			if err != nil {
				fmt.Println("Error sending pong:", err)
			}
		}
	}()

	wg.Wait()
}

func (ser *AgentServer) handleClient(conn net.Conn) {
	defer conn.Close()

	var clientId string
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.ClientId {
			log.Printf("Client Id Enter: %v", data)
			clientId = data
		} else if cmd == config.ClientData {
			fmt.Println("rpush", clientId, data)
			err := ser.redisCli.RPush(context.Background(), clientId, data).Err()
			if err != nil {
				log.Println("Failed to push values to Redis list:", err)
				return
			}
		} else if cmd == config.ClientExit {
			log.Printf("Client Id Exit: %v", clientId)
			break
		} else if cmd == config.FetchClientData {
			ser.fetchData(clientId, data)
		} else if cmd == config.FetchOldData {
		}
	}
}

func (ser *AgentServer) fetchData(clientId string, clusterIp string) {
	sockfile, conn := util.CreateMptcpConnection(clusterIp, config.DataTransferPort)
	defer sockfile.Close()
	util.SendNetMessage(conn, config.ClientId, clientId)
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.TransferData {
			ser.redisCli.RPush(context.Background(), clientId, data)
		} else if cmd == config.TransferEnd {
			log.Println("finish fetching data for client", clientId)
			break
		}
	}
}

func (ser *AgentServer) handleTransfer(conn net.Conn) {
	cmd, clientId := util.RecvNetMessage(conn)
	if cmd != config.ClientId {
		log.Println("Error: expected client id in the beginning of transfer")
	}
	result, err := ser.redisCli.LRange(context.Background(), clientId, 0, -1).Result()
	if err != nil {
		log.Println("Error:", err)
		return
	}
	for _, element := range result {
		util.SendNetMessage(conn, config.TransferData, element)
	}
	util.SendNetMessage(conn, config.TransferEnd, "")
}
