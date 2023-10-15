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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"

	"github.com/go-redis/redis/v8"
	externalnet "github.com/shirou/gopsutil/net"
)

type ServerInfo struct {
	transferIp  string
	serviceIp   string
	serviceName string
	proxyPort   int32
	pingPort    int32
}

type SenderBuffer struct {
	senderId      string
	priority      int
	triggerSendCh chan bool
	receiverId    string
}

// Record used for receiver to receive data from many senders
type SenderRecord struct {
	conn   net.Conn
	waitCh chan bool
}

type AgentServer struct {
	redisCli    *redis.Client
	myClusterIp string
	senderMap   map[string]SenderRecord
	bufferMap   map[string]SenderBuffer
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
		bufferMap:   make(map[string]SenderBuffer),
		k8sCli:      service.NewK8SClientInCluster(),
	}

	var wg sync.WaitGroup
	wg.Add(4)

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
		listener := util.CreateMptcpListener(config.MonitorNetPort)
		defer listener.Close()
		// Accept and handle client connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Failed to accept transfer connection:", err)
				continue
			}

			go ser.testService(conn)
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

	sbf, ok := ser.bufferMap[senderId]
	if !ok {
		log.Fatalf("sender %s record is not created in isFirstPriority", senderId)
	}

	maxPri := 0
	for cli, bf := range ser.bufferMap {
		if cli == senderId || bf.receiverId != sbf.receiverId {
			continue
		}
		if bf.priority > maxPri {
			maxPri = bf.priority
		}
	}
	return sbf.priority >= maxPri
}

func (ser *AgentServer) handleClient(conn net.Conn) {
	cmd, data := util.RecvNetMessage(conn)
	if cmd == config.TestServiceThrought {
		//util.SendNetMessage(conn, config.TestServiceThrought, "hello")
		//util.SendNetMessage(conn, config.TransferFinished, "")
		fmt.Println("智能代理收到测试命令")
		_, currClusterIp := util.RecvNetMessage(conn)
		ser.myClusterIp = currClusterIp
		_, serviceIp := util.RecvNetMessage(conn)
		//services := ser.updateServerInfo()
		message := "Hello,server!"
		//与其他智能代理建立连接
		sockfile, transferconn := util.CreateMptcpConnection(serviceIp, config.MonitorNetPort) //与智能代理建立tcp连接

		if transferconn == nil {
			log.Fatalln("Failed to create connection when create peer test conn")
		}
		//设置测试时间和发送字节数
		testDuration := 1 * time.Second
		bytesSent := 0
		bytesRecv := 0
		startTime := time.Now()

		for {
			if time.Since(startTime) > testDuration {
				fmt.Println("测试时间结束！！！")
				break
			}

			// 发送消息
			util.SendNetMessage(transferconn, config.TestBetweenServices, message)
			bytesSent += len(message) //记录在测试时间发送的字节数
		}
		util.SendNetMessage(transferconn, config.TransferEnd, "")
		elapsed := time.Since(startTime)
		for {
			cmd, data := util.RecvNetMessage(transferconn)
			if cmd == config.TransferData && data != "" {
				num, err := strconv.Atoi(data)
				if err != nil {
					fmt.Println("转换失败:", err)
				}
				bytesRecv = num
				sockfile.Close()
				transferconn.Close()
				break
			}
		}

		//发送测试结果给client
		util.SendNetMessage(conn, config.TestServiceThrought, fmt.Sprintf("%.3f-%.3f", float64(bytesSent)/(elapsed.Seconds()*1000000), float64(bytesSent-bytesRecv)/float64(bytesSent)))
		util.SendNetMessage(conn, config.TransferFinished, "")
		conn.Close()
	} else {
		cliId := data
		_, clientType := util.RecvNetMessage(conn)
		_, cliPriorityStr := util.RecvNetMessage(conn)
		priority, _ := strconv.Atoi(cliPriorityStr)
		_, currClusterIp := util.RecvNetMessage(conn)

		if ser.myClusterIp == "" {
			ser.myClusterIp = currClusterIp
			log.Printf("my cluster ip = %s\n", ser.myClusterIp)
		}
		_, prevClusterIp := util.RecvNetMessage(conn)
		// fetch old data
		if prevClusterIp != "" && prevClusterIp != ser.myClusterIp {
			for _, data := range ser.fetchData(cliId, prevClusterIp) {
				log.Println("rpush", cliId, data)
				ser.redisCli.RPush(context.Background(), cliId, data)
			}
		}
		log.Println(cliId, clientType, currClusterIp, prevClusterIp)
		util.SendNetMessage(conn, config.TransferFinished, "")

		if clientType == config.RoleSender {
			log.Println("serve for sender", cliId)

			_, receiverId := util.RecvNetMessage(conn)

			senderExit := false
			triggerSendCh := make(chan bool, 1)
			exitCh := make(chan bool, 1)
			exitWaitCh := make(chan bool, 1)
			bufferedData := []string{}
			var receiverClusterIp string = ""
			var transferConn net.Conn = nil

			ser.mu.Lock()
			ser.bufferMap[cliId] = SenderBuffer{
				senderId:      cliId,
				priority:      priority,
				triggerSendCh: triggerSendCh,
				receiverId:    receiverId,
			}
			ser.mu.Unlock()

			beginTransfer := func() {
				sockfile, tconn := util.CreateMptcpConnection(receiverClusterIp, config.DataTransferPort)
				transferConn = tconn
				if tconn == nil {
					log.Fatalln("Failed to create connection when create peer transfer conn")
				}
				defer sockfile.Close()
				util.SendNetMessage(transferConn, config.SendFreshData, "")
				util.SendNetMessage(transferConn, config.ClientId, cliId)
			}
			waitUntilConnCreated := func() {
				for {
					_, ok := ser.senderMap[cliId]
					if ok {
						break
					}
					time.Sleep(time.Millisecond * 10)
				}
			}
			endTransfer := func() {
				if receiverClusterIp == "" {
					log.Fatalln("receiver cluster ip is empty when sending data")
				}
				if receiverClusterIp != currClusterIp {
					if transferConn != nil {
						util.SendNetMessage(transferConn, config.TransferEnd, "")
					}
				} else {
					waitUntilConnCreated()
					util.SendNetMessage(ser.senderMap[cliId].conn, config.TransferEnd, cliId)
				}
			}
			sendBuffferedData := func() {
				if receiverClusterIp == "" {
					log.Fatalln("receiver cluster ip is empty when sending data")
				}
				if receiverClusterIp != currClusterIp {
					if transferConn == nil {
						beginTransfer()
					}
					for _, data := range bufferedData {
						util.SendNetMessage(transferConn, config.ClientData, data)
						log.Printf("send %s to %s\n", data, receiverClusterIp)
					}
				} else {
					waitUntilConnCreated()
					for _, data := range bufferedData {
						util.SendNetMessage(ser.senderMap[cliId].conn, config.ClientData, data)
					}
				}
				bufferedData = []string{}
			}
			transferData := func(data string) {
				if receiverClusterIp == "" {
					log.Fatalln("receiver cluster ip is empty when sending data")
				}
				if receiverClusterIp != currClusterIp {
					if transferConn == nil {
						beginTransfer()
					}
					log.Printf("send %s to %s\n", data, receiverClusterIp)
					util.SendNetMessage(transferConn, config.ClientData, data)
				} else {
					waitUntilConnCreated()
					util.SendNetMessage(ser.senderMap[cliId].conn, config.ClientData, data)
				}
			}

			// pull receiver cluster ip in a loop
			// wait until receiver connects into the cluster
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
						log.Printf("get receiver %s cluster ip: %s\n", receiverId, receiverClusterIp)
						select {
						case triggerSendCh <- true:
						default:
						}
						break
					}
				}
			}()

			// send buffered data loop
			go func() {
			sendloop:
				for {
					select {
					case <-triggerSendCh:
					case <-exitCh:
						break sendloop
					}
					if ser.isFirstPriority(cliId) {
						log.Println("send buffered data...")
						sendBuffferedData()
					}
					if senderExit {
						exitWaitCh <- true
					}
				}
			}()

			for {
				cmd, data := util.RecvNetMessage(conn)
				if cmd == config.ClientData {
					// if the peer hasn't connected into k8s, buffer the data first
					if receiverClusterIp == "" {
						log.Println("buffer data (receiver not connected):", data)
						bufferedData = append(bufferedData, data)
					} else {
						if ser.isFirstPriority(cliId) {
							for len(bufferedData) > 0 {
								time.Sleep(time.Millisecond * 10)
							}
							transferData(data)
						} else {
							// if not the first priority, buffer data
							log.Println("buffer data (not first priority):", data)
							bufferedData = append(bufferedData, data)
						}
					}
				} else if cmd == config.ClientExit {
					// sender disconnect before receiver connects
					senderExit = true
					if len(bufferedData) > 0 {
						<-exitWaitCh
					}
					endTransfer()
					select {
					case exitCh <- true:
					default:
					}
					ser.mu.Lock()
					delete(ser.bufferMap, cliId)
					ser.mu.Unlock()
					ser.triggerNextPriority(receiverId)
					log.Printf("sender %s Exit", cliId)
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
			log.Printf("%s recv from %d senders: %v\n", cliId, recvNum, senderIds)
			wg := sync.WaitGroup{}
			wg.Add(len(senderIds))
			for _, senderId := range senderIds {
				go func(senderId string) {
					defer wg.Done()
					log.Printf("set conn map [%s]\n", senderId)
					ch := make(chan bool)
					ser.mu.Lock()
					ser.senderMap[senderId] = SenderRecord{
						conn:   conn,
						waitCh: ch,
					}
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
		conn.Close()
	}
}

func (ser *AgentServer) testService(conn net.Conn) {
	//记录接收到的字节数
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()
	byteRecv := 0
	for {
		fmt.Println("接收端收到吞吐量测试命令")
		cmd, data := util.RecvNetMessage(conn)
		if cmd == config.TestBetweenServices {
			byteRecv += len(data)
			continue
		} else if cmd == config.TransferEnd {
			util.SendNetMessage(conn, config.TransferData, fmt.Sprintf("%d", byteRecv))
			log.Println("Service disconnected !!!")
			break
		}
	}
	conn.Close()
}

// server 获取智能代理的信息
func (ser *AgentServer) updateServerInfo() []ServerInfo {
	k8sServices := ser.k8sCli.GetNamespaceServices(config.Namespace)
	serverNum := len(k8sServices) / 2
	serverInfo := make([]ServerInfo, serverNum)
	for _, svc := range k8sServices {
		lastChar := svc.SvcName[len(svc.SvcName)-1]
		lastDigit, err := strconv.Atoi(string(lastChar))
		lastDigit--
		if err != nil {
			fmt.Println("Atoi Error:", err)
			continue
		}
		if strings.HasPrefix(svc.SvcName, config.ProxyServicePrefix) {
			serverInfo[lastDigit].serviceIp = svc.ClusterIp
			serverInfo[lastDigit].serviceName = svc.SvcName
			for _, portInfo := range svc.Ports {
				if portInfo.Name == "client-port" {
					serverInfo[lastDigit].proxyPort = portInfo.NodePort
				} else if portInfo.Name == "ping-port" {
					pingPort := portInfo.NodePort
					serverInfo[lastDigit].pingPort = pingPort
				}
			}
		} else if strings.HasPrefix(svc.SvcName, config.ClusterServicePrefix) {
			serverInfo[lastDigit].transferIp = svc.ClusterIp
		}
	}
	return serverInfo
}

func (ser *AgentServer) triggerNextPriority(receiverId string) {
	ser.mu.Lock()
	defer ser.mu.Unlock()

	var nextBf SenderBuffer
	maxPri := 0
	for _, bf := range ser.bufferMap {
		if bf.receiverId != receiverId {
			continue
		}
		if bf.priority > maxPri {
			nextBf = bf
			maxPri = bf.priority
		}
	}
	if maxPri > 0 {
		select {
		case nextBf.triggerSendCh <- true:
		default:
		}
		log.Println("trigger next priority:", nextBf.senderId)
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

func (ser *AgentServer) monitorThroughput(conn net.Conn) {
	defer conn.Close()

	//定义监控的时间间隔
	interval := 1 * time.Second
	_, statSend1, statrev1 := ser.getNetState()
	time.Sleep(interval)
	statName2, statSend2, statrev2 := ser.getNetState()

	//创建表格用于输出当前智能代理网络信息信息
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"网络接口接口", "接收字节", "发送字节", "当前吞吐量(Byte/s)"})

	//求每个网络接口的吞吐量
	for i := 0; i < len(statrev1); i++ {
		throught := (statrev2[i] + statSend2[i]) - (statrev1[i] + statSend1[i])
		if throught == 0 {
			continue
		}
		table.Append([]string{statName2[i], fmt.Sprintf("%d", statrev2[i]),
			fmt.Sprintf("%d", statrev2[i]), fmt.Sprintf("%d", throught)})
	}

	//输出表格
	table.Render()

}

func (ser *AgentServer) getNetState() (statName []string, statsend []int64, statrev []int64) {
	var sendNum, recvNum []int64
	var nameNum []string
	netStats, err := externalnet.IOCounters(true)
	if err != nil {
		fmt.Println("无法获取网络统计信息:", err)
		return nameNum, sendNum, recvNum
	}

	for _, stat := range netStats {
		nameNum = append(nameNum, stat.Name)
		sendNum = append(sendNum, int64(stat.BytesSent))
		recvNum = append(recvNum, int64(stat.BytesRecv))
	}
	return nameNum, sendNum, recvNum
}
