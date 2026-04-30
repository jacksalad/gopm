package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"gopm/internal/client"
	"gopm/internal/common"
	"gopm/internal/server"
)

func main() {
	mode := flag.String("mode", "", "server or client")
	port := flag.Int("port", 0, "server control port (server mode)")
	serverAddr := flag.String("server", "", "server control address (client mode)")
	local := flag.String("local", "", "local address or port (client mode)")
	mapPort := flag.Int("map", 0, "server exposed port (client mode)")
	token := flag.String("token", "", "authentication token")
	name := flag.String("name", "", "client name (client mode)")
	retry := flag.Bool("retry", false, "auto reconnect (client mode)")
	verbose := flag.Bool("verbose", false, "verbose logging")

	flag.Parse()

	common.SetVerbose(*verbose)

	switch *mode {
	case "server":
		if *port <= 0 {
			fmt.Fprintln(os.Stderr, "error: -port is required in server mode")
			flag.Usage()
			os.Exit(1)
		}
		s := server.NewServer(*port, *token, *verbose)
		if err := s.Run(); err != nil {
			common.Fatal("server error: %v", err)
		}

	case "client":
		if *serverAddr == "" || *local == "" || *mapPort <= 0 {
			fmt.Fprintln(os.Stderr, "error: -server, -local, and -map are required in client mode")
			flag.Usage()
			os.Exit(1)
		}
		localAddr := normalizeLocal(*local)
		cfg := client.ClientConfig{
			ServerAddr: *serverAddr,
			LocalAddr:  localAddr,
			MapPort:    *mapPort,
			Token:      *token,
			Name:       *name,
			Retry:      *retry,
			Verbose:    *verbose,
		}
		c := client.NewClient(cfg)
		if err := c.Run(); err != nil {
			common.Fatal("client error: %v", err)
		}

	default:
		fmt.Fprintln(os.Stderr, "error: -mode must be 'server' or 'client'")
		flag.Usage()
		os.Exit(1)
	}
}

// normalizeLocal converts "8080" to "127.0.0.1:8080".
func normalizeLocal(addr string) string {
	if _, err := strconv.Atoi(addr); err == nil {
		return "127.0.0.1:" + addr
	}
	return addr
}