package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	//	"sync"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/mesh/app"
	"github.com/skycoin/skycoin/src/mesh/messages"
	network "github.com/skycoin/skycoin/src/mesh/nodemanager"
)

var config = messages.GetConfig()

func main() {

	//messages.SetDebugLogLevel()
	messages.SetInfoLogLevel()

	var (
		size int
		err  error
	)

	args := os.Args
	if len(args) < 2 {
		printHelp()
		return
	}

	hopsStr := os.Args[1]
	hops, err := strconv.Atoi(hopsStr)
	if err != nil {
		fmt.Println("\nThe first argument should be a number of hops\n")
		return
	}

	if hops < 1 {
		fmt.Println("\nThe number of hops should be a positive number > 0\n")
		return
	}

	sizeStr := strings.ToLower(os.Args[2])

	kb := 1024
	mb := kb * kb
	gb := mb * kb

	if strings.HasSuffix(sizeStr, "mb") {
		sizemb := strings.TrimSuffix(sizeStr, "mb")
		size, err = strconv.Atoi(sizemb)
		if err != nil {
			fmt.Println("Incorrect number of megabytes:", sizemb)
			return
		}
		size *= mb
	} else if strings.HasSuffix(sizeStr, "gb") {
		sizegb := strings.TrimSuffix(sizeStr, "gb")
		size, err = strconv.Atoi(sizegb)
		if err != nil {
			fmt.Println("Incorrect number of gigabytes:", sizegb)
			return
		}
		size *= gb
	} else if strings.HasSuffix(sizeStr, "kb") {
		sizekb := strings.TrimSuffix(sizeStr, "kb")
		size, err = strconv.Atoi(sizekb)
		if err != nil {
			fmt.Println("Incorrect number of kilobytes:", sizekb)
			return
		}
		size *= kb
	} else if strings.HasSuffix(sizeStr, "b") {
		sizeb := strings.TrimSuffix(sizeStr, "b")
		size, err = strconv.Atoi(sizeb)
		if err != nil {
			fmt.Println("Incorrect number of bytes:", sizeb)
			return
		}
	} else {
		size, err = strconv.Atoi(sizeStr)
		if err != nil {
			fmt.Println("Incorrect number of bytes:", size)
			return
		}
		sizeStr += "b"
	}

	meshnet := network.NewNetwork()
	defer meshnet.Shutdown()

	clientAddr, serverAddr := meshnet.CreateSequenceOfNodes(hops + 1)

	server, err := echoServer(meshnet, serverAddr)
	if err != nil {
		panic(err)
	}

	client, err := app.NewClient(meshnet, clientAddr) // register client on the first node
	if err != nil {
		panic(err)
	}

	err = client.Dial(serverAddr) // client dials to server
	if err != nil {
		panic(err)
	}

	duration := benchmark(client, server, size)

	fmt.Println("server:", serverAddr.Hex())
	fmt.Println("client:", clientAddr.Hex())
	log.Println(sizeStr+" duration:", duration)
	log.Println("Ticks:", meshnet.GetTicks())
}

func benchmark(client *app.Client, server *app.Server, msgSize int) time.Duration {

	if msgSize < 1 {
		panic("message should be at least 1 byte")
	}

	msg := make([]byte, msgSize)

	start := time.Now()

	_, err := client.Send(msg)

	if err != nil {
		panic(err)
	}

	duration := time.Now().Sub(start)

	//time.Sleep(120 * time.Second)

	return duration
}

func echoServer(meshnet *network.NodeManager, serverAddr cipher.PubKey) (*app.Server, error) {

	srv, err := app.NewServer(meshnet, serverAddr, func(in []byte) []byte {
		return in
	})
	return srv, err
}

func printHelp() {
	fmt.Println("")
	fmt.Println("Usage: go run overall.go hops_number data_size\n")
	fmt.Println("Usage example:")
	fmt.Println("go run overall.go 40 100\t- 40 hops 100 bytes")
	fmt.Println("go run overall.go 200 100b\t- 200 hops 100 bytes")
	fmt.Println("go run overall.go 2 10kb\t- 2 hops 10 kilobytes")
	fmt.Println("go run overall.go 10 10mb\t- 10 hops 10 megabytes")
	fmt.Println("go run overall.go 50 1gb\t- 50 hops 1 gigabyte")
	fmt.Println("")
}
