package app

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/mesh/messages"
)

type VPNServer struct {
	proxyServer
	meshConns map[string]*Pipe
}

type Pipe struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

var (
	httpOk      = []byte("HTTP/1.1 200 OK\r\n" + "Server: A Go Web Server\r\n" + "Content-Type: text/plain; charset=utf-8\r\n" + "Content-Length: 0\r\n\r\n")
	httpMethods = map[string]bool{
		"GET":     true,
		"HEAD":    true,
		"POST":    true,
		"PUT":     true,
		"DELETE":  true,
		"CONNECT": true,
		"OPTIONS": true,
		// add anything if needed
	}
)

func NewVPNServer(meshnet messages.Network, address cipher.PubKey) (*VPNServer, error) {
	vpnServer := &VPNServer{}
	vpnServer.register(meshnet, address)
	vpnServer.lock = &sync.Mutex{}
	vpnServer.timeout = time.Duration(messages.GetConfig().AppTimeout)
	vpnServer.meshConns = map[string]*Pipe{}
	vpnServer.targetConns = map[string]net.Conn{}

	conn, err := meshnet.NewConnection(address)
	if err != nil {
		return nil, err
	}

	vpnServer.connection = conn

	err = meshnet.Register(address, vpnServer)
	if err != nil {
		return nil, err
	}

	log.Println("ready to accept requests")

	return vpnServer, nil
}

func (self *VPNServer) Consume(msg []byte) {

	proxyMessage := getProxyMessage(msg)
	if proxyMessage == nil {
		return
	}

	remoteAddr := proxyMessage.RemoteAddr // user address
	needClose := proxyMessage.NeedClose   // the message can be a comand to close the coresponding connection

	if needClose { // if there is a need to close a connection, close it if exists
		log.Printf("Closing connection %s according to a signal from client\n", remoteAddr)
		self.closeConns(remoteAddr)
		return
	}

	self.lock.Lock()
	meshConn, ok := self.meshConns[remoteAddr] // find the existing connection
	self.lock.Unlock()

	if !ok { // if there is no such connection create one
		fr, fw := io.Pipe()
		meshConn = &Pipe{reader: fr, writer: fw} // create a connection from meshnet to

		self.lock.Lock()
		self.meshConns[remoteAddr] = meshConn
		self.lock.Unlock()

		ready := make(chan bool)
		go self.serveConn(fr, remoteAddr, ready)
		<-ready
	}

	// write the data to the connection

	data := proxyMessage.Data

	_, err := meshConn.writer.Write(data)
	if err != nil { // if write is unsuccessful, close this connection and send the closing command to the corresponding client connection
		log.Println(err)
		self.sendClose(remoteAddr)
		self.closeConns(remoteAddr)
	}
}

func (self *VPNServer) serveConn(meshConn io.Reader, remoteAddr string, ready chan bool) {

	ready <- true
	request, err := getPacketFromConn(meshConn)
	if err != nil {
		log.Println(err)
	}

	typeIndex := bytes.IndexByte(request, 32)
	if typeIndex == -1 { // if no text then it is not a request, write the whole data to the target and exit
		err := self.writeToTarget(request, remoteAddr)
		log.Println(err)
		return
	}

	reqType := string(request[:typeIndex])

	if _, ok := httpMethods[reqType]; !ok { // if the first word is not a known method name then this is not a request, write the whole data to the target and exit
		err := self.writeToTarget(request, remoteAddr)
		log.Println(err)
		return
	}

	reqData := request[typeIndex+1:]

	urlIndex := bytes.IndexByte(reqData, 32)
	url := string(reqData[:urlIndex])

	urlData := strings.Split(url, "://")
	if len(urlData) > 1 {
		url = urlData[1]
	}

	urlelements := strings.Split(url, "/")
	fullhost := urlelements[0]

	fullhostwithauth := strings.Split(fullhost, "@")

	if len(fullhostwithauth) == 2 {
		fullhost = fullhostwithauth[1]
	}

	port := "80"
	hostdata := strings.Split(fullhost, ":")
	host := hostdata[0]
	if len(hostdata) == 2 {
		port = hostdata[1]
	}

	addr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		log.Println(err)
	} else {
		targetHost := addr.String() + ":" + port

		self.lock.Lock()
		existingTargetConn, ok := self.targetConns[remoteAddr]
		self.lock.Unlock()

		if ok {
			existingTargetConn.Close()
		}

		//creating connection to the target server
		targetConn, err := net.Dial("tcp", targetHost)
		if err != nil {
			self.sendClose(remoteAddr)
			self.closeConns(remoteAddr)
			log.Println(err)
		}

		self.lock.Lock()
		self.targetConns[remoteAddr] = targetConn
		self.lock.Unlock()

		if reqType == "CONNECT" {
			go io.Copy(targetConn, meshConn) // send requests to the server
			okMessage := messages.ProxyMessage{
				httpOk,
				remoteAddr,
				false,
			}
			okMsgS := messages.Serialize(messages.MsgProxyMessage, okMessage)
			self.Send(okMsgS)

		} else {

			self.lock.Lock()
			delete(self.meshConns, remoteAddr) // force the redial for the case of keep-alive with changing URLs
			self.lock.Unlock()

			err := self.writeToTarget(request, remoteAddr)
			if err != nil {
				log.Println(err)
			}
		}

		go self.getFromConn(targetConn, remoteAddr) // get replies from server
	}
}

func (self *VPNServer) writeToTarget(request []byte, remoteAddr string) error {
	self.lock.Lock()
	targetConn, ok := self.targetConns[remoteAddr]
	self.lock.Unlock()

	if !ok {
		return errors.New("Target connection not found for address:" + remoteAddr)
	}
	_, err := targetConn.Write(request)
	if err != nil {
		self.sendClose(remoteAddr)
		self.closeConns(remoteAddr)
	}
	return err
}

func (self *VPNServer) closeConns(remoteAddr string) {
	self.lock.Lock()
	delete(self.meshConns, remoteAddr)
	self.lock.Unlock()

	self.proxyServer.closeConns(remoteAddr)
}
