package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"syscall"
	"smart-agent/config"
	"smart-agent/util"
)

const (
	SOL_TCP       = 6
	MPTCP_ENABLED = 42
)

func main() {
	filePath := flag.String("file", "", "Path to the input file")
	clientId := flag.String("client", "", "Client ID")
	flag.Parse()

	// Check if the input file flag is provided
	if *filePath == "" {
		fmt.Println("Input file is required.")
		return
	}
	if *clientId == "" {
		fmt.Println("Client Id is required.")
		return
	}

	sockfile, conn := util.CreateMptcpConnection("127.0.0.1", config.ClientServePort)
	defer sockfile.Close()

	clientTask(conn, *filePath, *clientId)
}

func setMPTCPEnabled(sock int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(sock, SOL_TCP, MPTCP_ENABLED, 1))
}

func clientTask(conn net.Conn, filePath string, clientId string) {
	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	util.SendNetMessage(conn, config.ClientId, clientId)
	// Read the file line by line
	for scanner.Scan() {
		line := scanner.Text()
		util.SendNetMessage(conn, config.ClientData, line)
	}
	util.SendNetMessage(conn, config.ClientExit, "")
}
