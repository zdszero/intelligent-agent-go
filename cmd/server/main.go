package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"smart-agent/config"
	"smart-agent/service"
	"smart-agent/util"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type SenderRecord struct {
	receiverId string
	priority   int
	conn       net.Conn
	waitCh     chan bool
}

type AgentServer struct {
	redisCli    *redis.Client
	myClusterIp string
	senderMap   map[string]SenderRecord
	k8sCli      *service.K8SClient
	mu          sync.Mutex
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
		senderMap:   make(map[string]SenderRecord),
		k8sCli:      service.NewK8SClientInCluster(),
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
			_, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Println("Error reading message:", err)
				continue
			}

			// log.Printf("Received ping from %s: %s\n", addr.String(), string(buffer[:n]))

			_, err = conn.WriteToUDP([]byte("pong"), addr)
			if err != nil {
				log.Println("Error sending pong:", err)
			}
		}
	}()

	wg.Wait()
}

func checkCmdType(cmd uint32, target uint32) {
	if cmd != target {
		log.Fatalln("expected cmd type: %d, actual: %d", target, cmd)
	}
}

func (ser *AgentServer) isFirstPriority(senderId string) bool {
	ser.mu.Lock()
	defer ser.mu.Unlock()

	sr, ok := ser.senderMap[senderId]
	if !ok {
		log.Fatalf("sender %s record is not created in isFirstPriority", senderId)
	}

	maxPri := -1
	for cli, rec := range ser.senderMap {
		if cli == senderId || rec.receiverId != sr.receiverId {
			continue
		}
		if rec.priority > maxPri {
			maxPri = rec.priority
		}
	}
	if maxPri == -1 {
		return true
	}
	return sr.priority >= maxPri
}

func (ser *AgentServer) handleClient(conn net.Conn) {
	defer conn.Close()

	_, myClientId := util.RecvNetMessage(conn)
	_, clientType := util.RecvNetMessage(conn)
	_, cliPriorityStr := util.RecvNetMessage(conn)
	myPri, _ := strconv.Atoi(cliPriorityStr)
	_, myClusterIp := util.RecvNetMessage(conn)

	ser.mu.Lock()
	if ser.myClusterIp == "" {
		ser.myClusterIp = myClusterIp
		log.Printf("my cluster ip = %s\n", ser.myClusterIp)
	}
	sr, _ := ser.senderMap[myClientId]
	sr.priority = myPri
	ser.senderMap[myClientId] = sr
	ser.mu.Unlock()

	_, prevClusterIp := util.RecvNetMessage(conn)
	// fetch old data
	if prevClusterIp != "" && prevClusterIp != ser.myClusterIp {
		for _, data := range ser.fetchData(myClientId, prevClusterIp) {
			log.Println("rpush", myClientId, data)
			ser.redisCli.RPush(context.Background(), myClientId, data)
		}
	}
	log.Println(myClientId, clientType, myClusterIp, prevClusterIp)
	util.SendNetMessage(conn, config.TransferFinished, "")

	if clientType == config.RoleSender {
		log.Println("serve for sender", myClientId)
		bufferedData := []string{}
		var receiverClusterIp string = ""
		var transferConn net.Conn = nil

		_, receiverId := util.RecvNetMessage(conn)
		go func() {
			for {
				ip, err := ser.k8sCli.EtcdGet(receiverId)
				if err != nil {
					log.Fatalln("Failed to get receiver cluster ip:", err)
				}
				if ip == "" {
					time.Sleep(time.Millisecond * 300)
				} else {
					receiverClusterIp = ip
					break
				}
			}
		}()

		beginTransfer := func() {
			sockfile, tconn := util.CreateMptcpConnection(receiverClusterIp, config.DataTransferPort)
			transferConn = tconn
			if conn == nil {
				log.Fatalln("Failed to create connection when create peer transfer conn")
			}
			defer sockfile.Close()
			util.SendNetMessage(transferConn, config.SendFreshData, "")
			util.SendNetMessage(transferConn, config.ClientId, myClientId)
		}
		endTransfer := func() {
			if transferConn != nil {
				util.SendNetMessage(transferConn, config.TransferEnd, "")
			}
		}
		sendBuffferedData := func() {
			if transferConn == nil {
				beginTransfer()
			}
			if len(bufferedData) == 0 {
				return
			}
			for _, data := range bufferedData {
				util.SendNetMessage(transferConn, config.ClientData, data)
				log.Printf("send %s to %s\n", data, receiverClusterIp)
			}
			bufferedData = []string{}
		}
		transferData := func(data string) {
			if transferConn == nil {
				beginTransfer()
			}
			log.Printf("send %s to %s\n", data, receiverClusterIp)
			util.SendNetMessage(transferConn, config.ClientData, data)
		}

		for {
			cmd, data := util.RecvNetMessage(conn)
			if cmd == config.ClientData {
				// if the peer hasn't connected into k8s, buffer the data first
				if receiverClusterIp == "" {
					log.Println("buffer data (receiver not connected):", data)
					bufferedData = append(bufferedData, data)
				} else {
					if ser.isFirstPriority(myClientId) {
						sendBuffferedData()
						transferData(data)
					} else {
						// if not the first priority, buffer data
						log.Println("buffer data (not first priority):", data)
						bufferedData = append(bufferedData, data)
					}
				}
			} else if cmd == config.ClientExit {
				log.Printf("%s Exit", myClientId)
				// TODO: sender disconnect before receiver connects
				endTransfer()
				ser.mu.Lock()
				sr, _ := ser.senderMap[myClientId]
				sr.priority = 0
				ser.senderMap[myClientId] = sr
				ser.mu.Unlock()
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
	} else if clientType == config.RoleReceiver {
		_, recvNumStr := util.RecvNetMessage(conn)
		recvNum, _ := strconv.Atoi(recvNumStr)
		senderIds := []string{}
		for i := 0; i < recvNum; i++ {
			_, senderId := util.RecvNetMessage(conn)
			senderIds = append(senderIds, senderId)
		}
		log.Printf("%s recv from %d senders: %v\n", myClientId, recvNum, senderIds)
		wg := sync.WaitGroup{}
		wg.Add(len(senderIds))
		for _, senderId := range senderIds {
			go func(senderId string) {
				defer wg.Done()
				log.Printf("set conn map [%s]\n", senderId)
				ch := make(chan bool)
				ser.mu.Lock()
				sr, _ := ser.senderMap[senderId]
				sr.conn = conn
				sr.waitCh = ch
				ser.senderMap[senderId] = sr
				ser.mu.Unlock()

				<-ch

				ser.mu.Lock()
				delete(ser.senderMap, senderId)
				ser.mu.Unlock()
				log.Printf("sender %s finsihed\n", senderId)
			}(senderId)
		}
		wg.Wait()
	} else {
		log.Fatalln("unknown client type:", clientType)
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
	util.SendNetMessage(conn, config.FetchOldData, "")
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

	cmd, _ := util.RecvNetMessage(conn)
	_, clientId := util.RecvNetMessage(conn)
	if cmd == config.FetchOldData {
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
	} else if cmd == config.SendFreshData {
		for {
			cmd, data := util.RecvNetMessage(conn)
			if cmd == config.ClientData {
				log.Printf("relay data %s to receiver\n", data)
				ser.mu.Lock()
				util.SendNetMessage(ser.senderMap[clientId].conn, config.ClientData, data)
				ser.mu.Unlock()
				log.Println("rpush", clientId, data)
				ser.redisCli.RPush(context.Background(), clientId, data)
			} else if cmd == config.TransferEnd {
				log.Printf("relay end")
				ser.mu.Lock()
				sr := ser.senderMap[clientId]
				util.SendNetMessage(sr.conn, config.TransferEnd, clientId)
				sr.waitCh <- true
				ser.mu.Unlock()
				break
			}
		}
	}
}
