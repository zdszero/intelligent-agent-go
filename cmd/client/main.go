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
)

const (
	SOL_TCP       = 6
	MPTCP_ENABLED = 42
)

type AgentClient struct {
	clientId string
	conn     net.Conn
	k8sCli   service.K8SClient
	k8sSvc   []service.Service
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
		k8sCli: *service.NewK8SClient(""),
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
			eofCh<-true
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
			// TODO .connect serviceName
			arg := tokens[1]
			parts := strings.Split(arg, ":")
			ip := parts[0]
			port, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("Error:", err)
			}
			cli.connectTo(ip, int32(port))
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

func (cli *AgentClient) showService() {
	cli.k8sSvc = cli.k8sCli.GetNamespaceServices(config.AgentNamespace)
	for _, svc := range cli.k8sSvc {
		fmt.Printf("Service Name: %s\n", svc.SvcName)
		fmt.Printf("Cluster IP: %s\n", svc.ClusterIp)
		fmt.Printf("Ports:\n")
		for _, port := range svc.Ports {
			fmt.Printf("    Protocol: %s\n", port.Protocol)
			fmt.Printf("    Port: %d\n", port.Port)
			fmt.Printf("    Node Port: %d\n", port.NodePort)
			fmt.Printf("    Target Port: %s\n", port.TargetPort)
		}
		fmt.Println("--------------------")
	}
}

func (cli *AgentClient) connectTo(ip string, port int32) {
	sockfile, conn := util.CreateMptcpConnection("127.0.0.1", config.ClientServePort)
	defer sockfile.Close()

	if cli.conn != nil {
		util.SendNetMessage(cli.conn, config.ClientExit, "")
	}
	cli.conn = conn
	util.SendNetMessage(cli.conn, config.ClientId, cli.clientId)
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
