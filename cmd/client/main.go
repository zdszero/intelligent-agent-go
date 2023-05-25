package main

import (
	"bufio"
	"flag"
	"fmt"
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

type AgentClient struct {
	clientId      string
	conn          net.Conn
	k8sCli        service.K8SClient
	k8sSvc        []service.Service
	prevClusterIp string
}

func main() {
	clientId := flag.String("client", "", "Client ID")
	flag.Parse()

	// Check if the input file flag is provided
	if *clientId == "" {
		fmt.Println("Client Id is required.")
		return
	}

	cli := AgentClient{
		clientId:      *clientId,
		conn:          nil,
		k8sCli:        *service.NewK8SClient(""),
		prevClusterIp: "",
	}

	interruptChan := make(chan os.Signal, 1)
	eofCh := make(chan bool, 1)
	signal.Notify(interruptChan, syscall.SIGINT, syscall.SIGTERM)

	go repl(cli, eofCh)

	select {
	case <-interruptChan:
	case <-eofCh:
	}
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
			if cli.conn != nil {
				util.SendNetMessage(cli.conn, config.ClientExit, "")
			}
			eofCh <- true
			return
		case ".service":
			cli.showService()
		case ".connect":
			svcName := tokens[1]
			cli.connectToService(svcName)
			fmt.Println("successfully connected")
		case ".connectto":
			arg := tokens[1]
			parts := strings.Split(arg, ":")
			ip := parts[0]
			port, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("Error:", err)
			}
			cli.connectToIpPort(ip, int32(port))
			fmt.Println("successfully connected")
		case ".send":
			if cli.conn != nil {
				util.SendNetMessage(cli.conn, config.ClientData, tokens[1])
			}
		case ".sendfile":
			cli.sendFile(tokens[1])
		case ".ping":
			svcName := tokens[1]
			cli.pingServer(svcName)
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

func (cli *AgentClient) pullService() {
	cli.k8sSvc = cli.k8sCli.GetNamespaceServices(config.Namespace)
}

func (cli *AgentClient) showService() {
	cli.pullService()
	headers := []string{"Service Name", "ClusterIp", "Delay"}
	fmt.Printf("%-20s %-20s %-15s\n", headers[0], headers[1], headers[2])
	fmt.Println(strings.Repeat("-", 55))
	for _, svc := range cli.k8sSvc {
		fmt.Printf("%-20s %-20s %-15s\n", svc.SvcName, svc.ClusterIp, "")
	}
}

func (cli *AgentClient) connectToService(svcName string) {
	found := false
	var nodePort int32
	var clusterIp string
svcloop:
	for _, svc := range cli.k8sSvc {
		if svc.SvcName == svcName {
			for _, portInfo := range svc.Ports {
				if portInfo.Name == "client-port" {
					found = true
					nodePort = portInfo.NodePort
					clusterIp = svc.ClusterIp
					break svcloop
				}
			}
		}
	}
	if !found {
		fmt.Printf("service %s does not exist, please choose a valid service", svcName)
		return
	}

	cli.connectToIpPort(config.KubernetesIp, nodePort)
	cli.prevClusterIp = clusterIp
	cli.k8sCli.EtcdPut(cli.clientId, clusterIp)
}

func (cli *AgentClient) connectToIpPort(ip string, port int32) {
	fmt.Printf("connect to %s:%d", ip, port)

	sockfile, conn := util.CreateMptcpConnection(ip, port)
	defer sockfile.Close()

	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientExit, "")
	}
	cli.conn = conn
	util.SendNetMessage(cli.conn, config.ClientId, cli.clientId)
	util.SendNetMessage(cli.conn, config.ClusterIp, cli.prevClusterIp)
	finished, _ := util.RecvNetMessage(cli.conn)
	if finished != config.TransferFinished {
		fmt.Println("Fail to receive TransferFinished after sending ClientId")
		os.Exit(1)
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

func (cli *AgentClient) pingServer(svcName string) {

	found := false
	var nodePort int32
svcloop:
	for _, svc := range cli.k8sSvc {
		if svc.SvcName == svcName {
			for _, portInfo := range svc.Ports {
				if portInfo.Name == "ping-port" {
					found = true
					nodePort = portInfo.NodePort
					break svcloop
				}
			}
		}
	}
	if !found {
		fmt.Printf("service %s does not exist, please choose a valid service", svcName)
		return
	}

	fmt.Printf("connect to %s:%d", config.KubernetesIp, nodePort)
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", config.KubernetesIp, nodePort))
	if err != nil {
		fmt.Println("Error resolving server address:", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	message := []byte("ping")
	start := time.Now()

	_, err = conn.Write(message)
	if err != nil {
		fmt.Println("Error sending ping:", err)
		return
	}

	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) // Set read timeout

	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Println("Error receiving pong:", err)
		return
	}

	elapsed := time.Since(start)
	fmt.Println("Ping response:", string(buffer))
	fmt.Println("Round trip time:", elapsed)
}

func (cli *AgentClient) fetchClientData(clientId string) {
	clusterIp, err := cli.k8sCli.EtcdGet(clientId)
	if err != nil {
		fmt.Printf("Failed to fetch %s's clusterIp: %v", clientId, clusterIp)
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
