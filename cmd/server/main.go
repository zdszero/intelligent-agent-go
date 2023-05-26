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

	ser := AgentServer{
		redisCli:    redisCli,
		myClusterIp: "",
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

func (ser *AgentServer) handleClient(conn net.Conn) {
	defer conn.Close()

	fmt.Println("handle connection", conn)
	var clientId string
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.ClientId {
			log.Printf("%s Enter", data)
			clientId = data
			_, myClusterIp := util.RecvNetMessage(conn)
			if ser.myClusterIp == "" {
				ser.myClusterIp = myClusterIp
				log.Printf("my cluster ip = %s\n", ser.myClusterIp)
			}
			_, prevClusterIp := util.RecvNetMessage(conn)
			log.Printf("%s previous cluster ip: %s\n", clientId, prevClusterIp)
			if prevClusterIp != "" && prevClusterIp != ser.myClusterIp {
				for _, data := range ser.fetchData(clientId, prevClusterIp) {
					log.Println("rpush", clientId, data)
					ser.redisCli.RPush(context.Background(), clientId, data)
				}
			}
			log.Println("sending transfer finish to client")
			util.SendNetMessage(conn, config.TransferFinished, "")
		} else if cmd == config.ClientData {
			log.Println("rpush", clientId, data)
			err := ser.redisCli.RPush(context.Background(), clientId, data).Err()
			if err != nil {
				log.Println("Failed to push values to Redis list:", err)
				return
			}
		} else if cmd == config.ClientExit {
			log.Printf("%s Exit", clientId)
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
	if conn == nil {
		log.Println("Failed to create connection when fetching data")
		return []string{}
	}
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

func (ser *AgentServer) handleTransfer(conn net.Conn) {
	defer conn.Close()

	log.Println("handle transfer:", conn)
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
