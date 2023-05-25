package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"smart-agent/config"
	"smart-agent/util"
	"sync"

	"github.com/go-redis/redis/v8"
)

type AgentServer struct {
	redisCli    *redis.Client
	myClusterIp string
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

	clusterIP := getClusterIp()
	log.Println("agent server cluster ip:", clusterIP)
	ser := AgentServer{
		redisCli:    redisCli,
		myClusterIp: clusterIP,
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		listener := util.CreateMptcpListener(config.ClientServePort)
		// defer listener.Close()
		// Accept and handle client connections
		for {
			conn, err := listener.Accept()
			log.Println("Accept connection from:", conn)
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
			log.Println("Error resolving server address:", err)
			return
		}

		conn, err := net.ListenUDP("udp", serverAddr)
		if err != nil {
			log.Println("Error listening:", err)
			return
		}
		defer conn.Close()

		buffer := make([]byte, 1024)

		for {
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Println("Error reading message:", err)
				continue
			}

			log.Printf("Received ping from %s: %s\n", addr.String(), string(buffer[:n]))

			_, err = conn.WriteToUDP([]byte("pong"), addr)
			if err != nil {
				log.Println("Error sending pong:", err)
			}
		}
	}()

	wg.Wait()
}

func getClusterIp() string {
	var clusterIp string
	for i := 1; i <= 20; i++ {
		clusterIp = os.Getenv(fmt.Sprintf("MY_AGENT_SERVICE%d_SERVICE_HOST", i))
		if clusterIp != "" {
			break
		}
	}
	return clusterIp
}

func (ser *AgentServer) handleClient(conn net.Conn) {
	defer conn.Close()

	var clientId string
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.ClientId {
			log.Printf("Client %s Enter", data)
			clientId = data
			_, clusterIp := util.RecvNetMessage(conn)
			if clusterIp != "" && clusterIp != ser.myClusterIp {
				ser.pushFetchedData(clientId, clusterIp)
			}
			util.SendNetMessage(conn, config.TransferFinished, "")
		} else if cmd == config.ClientData {
			log.Println("rpush", clientId, data)
			err := ser.redisCli.RPush(context.Background(), clientId, data).Err()
			if err != nil {
				log.Println("Failed to push values to Redis list:", err)
				return
			}
		} else if cmd == config.ClientExit {
			log.Printf("Client %s Exit", clientId)
			break
		} else if cmd == config.FetchClientData {
			targetClientId := data
			_, targetClusterIp := util.RecvNetMessage(conn)
			for _, data := range ser.fetchData(targetClientId, targetClusterIp) {
				util.SendNetMessage(conn, config.TransferData, data)
			}
			util.SendNetMessage(conn, config.TransferEnd, "")
		}
	}
}

func (ser *AgentServer) fetchData(clientId string, clusterIp string) []string {
	if clusterIp == ser.myClusterIp {
		result, err := ser.redisCli.LRange(context.Background(), clientId, 0, -1).Result()
		if err != nil {
			log.Println("Error during redis lrange:", err)
			return []string{}
		}
		return result
	}
	log.Printf("start fetching data for %s from %s:%d\n", clientId, clusterIp, config.DataTransferPort)
	sockfile, conn := util.CreateMptcpConnection(clusterIp, config.DataTransferPort)
	defer sockfile.Close()
	util.SendNetMessage(conn, config.ClientId, clientId)
	dataset := []string{}
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.TransferData {
			dataset = append(dataset, data)
		} else if cmd == config.TransferEnd {
			break
		}
	}
	log.Printf("finish fetching data for %s from %s\n", clientId, clusterIp)
	return dataset
}

func (ser *AgentServer) pushFetchedData(clientId string, clusterIp string) {
	for _, data := range ser.fetchData(clientId, clusterIp) {
		ser.redisCli.RPush(context.Background(), clientId, data)
	}
}

func (ser *AgentServer) handleTransfer(conn net.Conn) {
	cmd, clientId := util.RecvNetMessage(conn)
	if cmd != config.ClientId {
		log.Fatalln("Error: expected client id in the beginning of transfer")
	}
	log.Printf("Send %s data to %s\n", clientId, conn.LocalAddr().String())
	result, err := ser.redisCli.LRange(context.Background(), clientId, 0, -1).Result()
	if err != nil {
		log.Println("Error during redis lrange:", err)
		return
	}
	for _, element := range result {
		util.SendNetMessage(conn, config.TransferData, element)
	}
	util.SendNetMessage(conn, config.TransferEnd, "")
	keysDel, err := ser.redisCli.Del(context.Background(), clientId).Result()
	if err != nil {
		log.Println("Failed to delete list", clientId)
	}
	log.Printf("Delete list %s, number of keys deleted: %d\n", clientId, keysDel)
	log.Printf("Send %s data finished\n", clientId)
}
