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
	clientId     string
	conn         net.Conn
	k8sCli       service.K8SClient
	k8sSvc       []service.Service
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
		clientId: *clientId,
		conn:     nil,
		k8sCli:   *service.NewK8SClient(""),
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
			return
		case ".service":
			cli.showService()
		case ".connect":
			svcName := tokens[1]
			cli.connectTo(svcName)
			fmt.Println("successfully connected")
		case ".send":
			if cli.conn != nil {
				util.SendNetMessage(cli.conn, config.ClientData, tokens[1])
			}
		case ".sendfile":
			cli.sendFile(tokens[1])
		case ".fetch":
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
	for _, svc := range cli.k8sSvc {
		fmt.Printf("Service Name: %s\n", svc.SvcName)
		fmt.Printf("Cluster IP: %s\n", svc.ClusterIp)
		fmt.Printf("Ports:\n")
		for _, port := range svc.Ports {
			fmt.Printf("    Name: %s\n", port.Name)
			fmt.Printf("    Protocol: %s\n", port.Protocol)
			fmt.Printf("    Port: %d\n", port.Port)
			fmt.Printf("    Node Port: %d\n", port.NodePort)
			fmt.Printf("    Target Port: %s\n", port.TargetPort)
		}
		fmt.Println("--------------------")
	}
}

func (cli *AgentClient) connectTo(svcName string) {
	found := false
	var nodePort int32
svcloop:
	for _, svc := range cli.k8sSvc {
		if svc.SvcName == svcName {
			for _, portInfo := range svc.Ports {
				if portInfo.Name == "client-port" {
					found = true
					nodePort = portInfo.NodePort
					break svcloop
				}
			}
		}
	}
	if !found {
		fmt.Printf("service %s does not exist, please choose a valid service")
		return
	}

	sockfile, conn := util.CreateMptcpConnection(config.KubernetesIp, nodePort)
	defer sockfile.Close()

	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientExit, "")
	}
	cli.conn = conn
	util.SendNetMessage(cli.conn, config.ClientId, cli.clientId)
	finished, _ := util.RecvNetMessage(cli.conn)
	if finished != config.TransferFinished {
		fmt.Errorf("Fail to receive TransferFinished after sending ClientId")
		os.Exit(1)
	}
	cli.k8sCli.EtcdPut(cli.clientId, svcName)
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

func pingServer(ip string, port int32) {
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ip, port))
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
