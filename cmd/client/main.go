package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"smart-agent/config"
	"smart-agent/service"
	"smart-agent/util"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ServerInfo struct {
	transferIp  string
	serviceIp   string
	serviceName string
	proxyPort   int32
	pingPort    int32
	delay       time.Duration
}

type AgentClient struct {
	clientId      string
	conn          net.Conn
	k8sCli        service.K8SClient
	k8sSvc        []service.Service
	serverInfo    []ServerInfo
	prevClusterIp string
	currClusterIp string
}

func main() {
	clientId := flag.String("client", "", "Client ID")
	flag.Parse()

	// Check if the input file flag is provided
	if *clientId == "" {
		fmt.Println("Client Id is required.")
		return
	}

	cli := newAgentClient(*clientId)

	interruptChan := make(chan os.Signal, 1)
	eofCh := make(chan bool, 1)
	signal.Notify(interruptChan, syscall.SIGINT, syscall.SIGTERM)

	go servicePoller(cli)
	go repl(cli, eofCh)

	select {
	case <-interruptChan:
	case <-eofCh:
	}
}

func newAgentClient(clientId string) AgentClient {
	cli := AgentClient{
		clientId:      clientId,
		conn:          nil,
		k8sCli:        *service.NewK8SClient(""),
		prevClusterIp: "",
	}
	cli.updateServerInfo()
	return cli
}

func repl(cli AgentClient, eofCh chan bool) {
	fmt.Println("Welcome to Client REPL! Type '.help' for available commands.")
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			eofCh <- true
			break
		}
		command := scanner.Text()

		tokens := strings.Split(command, " ")
		if len(tokens) > 2 {
			fmt.Println("Invalid command. Type '.help' for available commands.")
			continue
		}
		cmd := tokens[0]
		switch cmd {
		case ".help":
			printHelp()
		case ".exit":
			cli.disconnect()
			eofCh <- true
			return
		case ".service":
			cli.showService()
		case ".connect":
			svcName := tokens[1]
			cli.connectToService(svcName)
			fmt.Println("successfully connected")
		case ".connectto":
			// not exposed to help, only for debugging
			arg := tokens[1]
			parts := strings.Split(arg, ":")
			ip := parts[0]
			port, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("Error:", err)
			}
			cli.debugConnect(ip, int32(port))
			fmt.Println("successfully connected")
		case ".send":
			cli.sendData(tokens[1])
		case ".sendfile":
			cli.sendFile(tokens[1])
		case ".fetch":
			var fetchClient string
			if len(tokens) == 1 {
				fetchClient = cli.clientId
			} else {
				fetchClient = tokens[1]
			}
			cli.fetchClientData(fetchClient)
		default:
			fmt.Println("Unknown command. Type '.help' for available commands.")
		}
	}
}

func printHelp() {
	help := fmt.Sprintf(
		`Usage:
    %s <command> [arguments]
The commands and arguments are:
    .help
    .exit
    .service
    .connect  [serviceName]
    .send     [data]
    .sendfile [filePath]
    .fetch    [clientId]
`, os.Args[0])
	fmt.Println(help)
}

func servicePoller(cli AgentClient) {
	for {
		time.Sleep(time.Second * 10)
		cli.updateServerInfo()
	}
}

func (cli *AgentClient) updateServerInfo() {
	cli.k8sSvc = cli.k8sCli.GetNamespaceServices(config.Namespace)
	serverNum := len(cli.k8sSvc) / 2
	serverInfo := make([]ServerInfo, serverNum)
	for _, svc := range cli.k8sSvc {
		lastChar := svc.SvcName[len(svc.SvcName)-1]
		lastDigit, err := strconv.Atoi(string(lastChar))
		lastDigit--
		if err != nil {
			log.Println("Atoi Error:", err)
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
					serverInfo[lastDigit].delay, err = cli.getPingDelay(pingPort)
					if err != nil {
						fmt.Println("fail to ping server on port %d\n", pingPort)
					}
				}
			}
		} else if strings.HasPrefix(svc.SvcName, config.ClusterServicePrefix) {
			serverInfo[lastDigit].transferIp = svc.ClusterIp
		}
	}
	cli.serverInfo = serverInfo
}

func (cli *AgentClient) showService() {
	cli.updateServerInfo()
	headers := []string{"Service Name", "Proxy IP", "Delay"}
	fmt.Printf("%-20s %-25s %-15s\n", headers[0], headers[1], headers[2])
	fmt.Println(strings.Repeat("-", 60))
	for _, info := range cli.serverInfo {
		fmt.Printf("%-20s %-25s %-15s\n", info.serviceName, fmt.Sprintf("%s:%d", config.KubernetesIp, info.proxyPort),
			fmt.Sprintf("%dms", info.delay.Abs().Microseconds()))
	}
}

func (cli *AgentClient) sendData(data string) {
	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientData, data)
	}
}

func (cli *AgentClient) findTransferIp(svcName string) string {
	var transferIp string
	for _, info := range cli.serverInfo {
		if info.serviceName == svcName {
			transferIp = info.transferIp
		}
	}
	return transferIp
}

func (cli *AgentClient) findProxyPort(svcName string) int32 {
	var port int32
	for _, info := range cli.serverInfo {
		if info.serviceName == svcName {
			port = info.proxyPort
		}
	}
	return port
}

func (cli *AgentClient) findPingPort(svcName string) int32 {
	var port int32
	for _, info := range cli.serverInfo {
		if info.serviceName == svcName {
			port = info.pingPort
		}
	}
	return port
}

func (cli *AgentClient) getPingDelay(port int32) (time.Duration, error) {
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", config.KubernetesIp, port))
	if err != nil {
		fmt.Println("Error resolving server address:", err)
		return 0, err
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return 0, err
	}
	defer conn.Close()

	message := []byte("ping")
	start := time.Now()

	_, err = conn.Write(message)
	if err != nil {
		fmt.Println("Error sending ping:", err)
		return 0, err
	}

	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) // Set read timeout

	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Println("Error receiving pong:", err)
		return 0, err
	}

	elapsed := time.Since(start)
	return elapsed, nil
}

func (cli *AgentClient) connectToService(svcName string) {
	proxyPort := cli.findProxyPort(svcName)
	clusterIp := cli.findTransferIp(svcName)

	cli.prevClusterIp = cli.currClusterIp
	cli.currClusterIp = clusterIp
	fmt.Printf("change cluster ip from %s to %s\n", cli.prevClusterIp, cli.currClusterIp)
	cli.connectToIpPort(config.KubernetesIp, proxyPort)
	cli.k8sCli.EtcdPut(cli.clientId, clusterIp)
}

// used for local debugging
func (cli *AgentClient) debugConnect(ip string, port int32) {
	cli.prevClusterIp = cli.currClusterIp
	cli.currClusterIp = ip
	fmt.Printf("change cluster ip from %s to %s\n", cli.prevClusterIp, cli.currClusterIp)
	cli.connectToIpPort(ip, port)
}

func (cli *AgentClient) connectToIpPort(ip string, port int32) {
	fmt.Printf("connect to %s:%d\n", ip, port)

	sockfile, conn := util.CreateMptcpConnection(ip, port)
	// TODO: handle conn == nil
	defer sockfile.Close()

	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientExit, "")
		cli.conn.Close()
	}
	fmt.Printf("send clientid %s to new conn\n", cli.clientId)
	util.SendNetMessage(conn, config.ClientId, cli.clientId)
	fmt.Printf("send curr cluster ip %s to new conn\n", cli.currClusterIp)
	util.SendNetMessage(conn, config.ClusterIp, cli.currClusterIp)
	fmt.Printf("send previous cluster ip %s to new conn\n", cli.prevClusterIp)
	util.SendNetMessage(conn, config.ClusterIp, cli.prevClusterIp)
	finished, _ := util.RecvNetMessage(conn)
	if finished != config.TransferFinished {
		fmt.Println("Fail to receive TransferFinished after sending ClientId")
		os.Exit(1)
	}
	fmt.Println("server has fetched old data")
	cli.conn = conn
}

func (cli *AgentClient) disconnect() {
	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientExit, "")
		cli.conn.Close()
	}
}

func (cli *AgentClient) sendFile(filePath string) {
	if cli.conn == nil {
		return
	}
	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Read the file line by line
	for scanner.Scan() {
		line := scanner.Text()
		util.SendNetMessage(cli.conn, config.ClientData, line)
	}
}

func (cli *AgentClient) fetchClientData(clientId string) {
	clusterIp, err := cli.k8sCli.EtcdGet(clientId)
	if err != nil {
		fmt.Printf("Failed to fetch %s's clusterIp: %v\n", clientId, err)
		return
	}
	util.SendNetMessage(cli.conn, config.FetchClientData, clientId)
	util.SendNetMessage(cli.conn, config.ClusterIp, clusterIp)
	dataset := []string{}
	for {
		cmd, data := util.RecvNetMessage(cli.conn)
		if cmd == config.TransferData {
			dataset = append(dataset, data)
		} else if cmd == config.TransferEnd {
			break
		}
	}
	fmt.Printf("%s data:\n", clientId)
	for _, data := range dataset {
		fmt.Println(data)
	}
}
