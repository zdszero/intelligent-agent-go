package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"smart-agent/config"
	"smart-agent/service"
	"smart-agent/util"
	"sync"

	"github.com/go-redis/redis/v8"
)

type AgentServer struct {
	redisCli         *redis.Client
	k8sCli           service.K8SClient
	k8sSvc           []service.Service
	mySvcName string
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

	serviceName := os.Getenv("KUBERNETES_SERVICE_NAME")
	clusterIP := os.Getenv("KUBERNETES_SERVICE_HOST")
	ser := AgentServer{
		redisCli:    redisCli,
		k8sCli:      *service.NewK8SClient(""),
		k8sSvc:      []service.Service{},
		mySvcName:   serviceName,
		myClusterIp: clusterIP,
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

func (ser *AgentServer) pollService() {
	ser.k8sSvc = ser.k8sCli.GetNamespaceServices(config.Namespace)
}

func (ser *AgentServer) handleClient(conn net.Conn) {
	defer conn.Close()

	var clientId string
	for {
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.ClientId {
			log.Printf("Client Id Enter: %v", data)
			clientId = data
			// try to fetch client's old data
			ser.fetchData(clientId)
			util.SendNetMessage(conn, config.TransferFinished, "")
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
			ser.fetchData(clientId)
		}
	}
}

func (ser *AgentServer) fetchData(clientId string) {
	ser.pollService()
	cliPrevSvc, err := ser.k8sCli.EtcdGet(clientId)
	if err != nil {
		log.Printf("Cannot fetch %s's data", clientId)
		return
	}

	if cliPrevSvc == ser.mySvcName || cliPrevSvc == "" {
		return
	}

	var clusterIp string
	found := false
	for _, svc := range ser.k8sSvc {
		if svc.SvcName == cliPrevSvc {
			found = true
			break
		}
	}
	if !found {
		return
	}

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
	keysDel, err := ser.redisCli.Del(context.Background(), clientId).Result()
	if err != nil {
		log.Println("Failed to delete list", clientId)
	}
	log.Printf("Delete list %s, number of keys deleted: %d\n", clientId, keysDel)
}
