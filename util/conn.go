package util

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	SOL_TCP       = 6
	MPTCP_ENABLED = 42
)

func CreateMptcpListener(port int32) net.Listener {
	// Create a TCP listener socket
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Println("Failed to create listener:", err)
		return nil
	}

	// Enable MPTCP on the listener socket
	file, err := listener.(*net.TCPListener).File()
	if err != nil {
		log.Println("Failed to get file descriptor:", err)
		return nil
	}
	sock := int(file.Fd())
	err = setMPTCPEnabled(sock)
	if err != nil {
		log.Println("use tcp:", err)
	} else {
		log.Println("use mptcp")
	}
	return listener
}

// NOTE: use defer file.Close() after this function
func CreateMptcpConnection(ip string, port int32) (*os.File, net.Conn) {
	// Create a socket
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		fmt.Println("Failed to create socket:", err)
		return nil, nil
	}
	defer syscall.Close(fd)

	// Enable MPTCP on the socket
	err = setMPTCPEnabled(fd)
	if err != nil {
		fmt.Println("use tcp:", err)
	} else {
		fmt.Println("use mptcp")
	}

	tokens := strings.Split(ip, ".")
	if len(tokens) != 4 {
		fmt.Println("Error address is not valid ipv4")
		return nil, nil
	}
	var addr [4]byte
	for i, tok := range tokens {
		num, err := strconv.Atoi(tok)
		if err != nil {
			fmt.Printf("Error converting string to integer: %v\n", err)
			return nil, nil
		}
		addr[i] = byte(num)
	}
	// Prepare the server address
	serverAddr := &syscall.SockaddrInet4{
		Port: int(port), // Server port
		Addr: addr,  // Server IP address
	}

	// Connect to the server
	err = syscall.Connect(fd, serverAddr)
	if err != nil {
		fmt.Println("Failed to connect:", err)
		return nil, nil
	}

	// Convert the sockfile descriptor to a net.Conn
	sockfile := os.NewFile(uintptr(fd), "")
	defer sockfile.Close()

	conn, err := net.FileConn(sockfile)
	if err != nil {
		fmt.Println("Failed to create net.Conn:", err)
		return nil, nil
	}
	return sockfile, conn
}

func setMPTCPEnabled(sock int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(sock, SOL_TCP, MPTCP_ENABLED, 1))
}

