package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/gui"
	"github.com/skycoin/skycoin/src/util/browser"
	"github.com/skycoin/skycoin/src/util/cert"
	"github.com/skycoin/skycoin/src/util/file"
	"github.com/skycoin/skycoin/src/util/logging"
)

var (
	// Version node version which will be set when build wallet by LDFLAGS
	Version    = "0.0.0"
	logger     = logging.MustGetLogger("main")
	coinName   = "shellcoin"
	logFormat  = "[shellcoin.%{module}:%{level}] %{message}"
	logModules = []string{
		"main",
		"daemon",
		"coin",
		"gui",
		"util",
		"visor",
		"wallet",
		"gnet",
		"pex",
		"webrpc",
	}

	//TODO: Move time and other genesis block settigns from visor, to here
	GenesisSignatureStr = "133067c26b92641433dd6be12c3898d14646a07a29fe51547c69f76da0bbfd2973aa48d4cb41c282866c1bda09a979c2ccd9fd53ad8ac98fcbe9033d53bb75eb01"
	GenesisAddressStr   = "EmQkiYpw14SHHkVFVeMqnPouPERKtvtF1A"
	BlockchainPubkeyStr = "02af0b8addc4e0be5922e98a1d8ebd91cf5f034ccd8756f126f9714507fd178a78"
	BlockchainSeckeyStr = ""

	GenesisTimestamp  uint64 = 1489844528
	GenesisCoinVolume uint64 = 300e12

	//GenesisTimestamp: 1426562704,
	//GenesisCoinVolume: 100e12, //100e6 * 10e6

	DefaultConnections = []string{
		"120.55.114.17:7100",
		"119.23.146.83:7100",
	}
)

// Command line interface arguments

type Config struct {
	// Disable peer exchange
	DisablePEX bool
	// Don't make any outgoing connections
	DisableOutgoingConnections bool
	// Don't allowing incoming connections
	DisableIncomingConnections bool
	// Disables networking altogether
	DisableNetworking bool
	// Only run on localhost and only connect to others on localhost
	LocalhostOnly bool
	// Which address to serve on. Leave blank to automatically assign to a
	// public interface
	Address string
	//gnet uses this for TCP incoming and outgoing
	Port int
	//max connections to maintain
	MaxConnections int
	// How often to make outgoing connections
	OutgoingConnectionsRate time.Duration
	// Wallet Address Version
	//AddressVersion string
	// Remote web interface
	WebInterface      bool
	WebInterfacePort  int
	WebInterfaceAddr  string
	WebInterfaceCert  string
	WebInterfaceKey   string
	WebInterfaceHTTPS bool

	RPCInterface     bool
	RPCInterfacePort int
	RPCInterfaceAddr string

	// Launch System Default Browser after client startup
	LaunchBrowser bool

	// If true, print the configured client web interface address and exit
	PrintWebInterfaceAddress bool

	// Data directory holds app data -- defaults to ~/.skycoin
	DataDirectory string
	// GUI directory contains assets for the html gui
	GUIDirectory string
	// Logging
	ColorLog bool
	// This is the value registered with flag, it is converted to LogLevel after parsing
	LogLevel string

	// Wallets
	// Defaults to ${DataDirectory}/wallets/
	WalletDirectory string

	RunMaster bool

	GenesisSignature cipher.Sig
	GenesisTimestamp uint64
	GenesisAddress   cipher.Address

	BlockchainPubkey cipher.PubKey
	BlockchainSeckey cipher.SecKey

	/* Developer options */

	// Enable cpu profiling
	ProfileCPU bool
	// Where the file is written to
	ProfileCPUFile string
	// HTTP profiling interface (see http://golang.org/pkg/net/http/pprof/)
	HTTPProf bool
	// Will force it to connect to this ip:port, instead of waiting for it
	// to show up as a peer
	ConnectTo string

	DBPath       string
	Arbitrating  bool
	RPCThreadNum uint // rpc number
	Logtofile    bool
}

func (c *Config) register() {
	flag.BoolVar(&c.DisablePEX, "disable-pex", c.DisablePEX,
		"disable PEX peer discovery")
	flag.BoolVar(&c.DisableOutgoingConnections, "disable-outgoing",
		c.DisableOutgoingConnections, "Don't make outgoing connections")
	flag.BoolVar(&c.DisableIncomingConnections, "disable-incoming",
		c.DisableIncomingConnections, "Don't make incoming connections")
	flag.BoolVar(&c.DisableNetworking, "disable-networking",
		c.DisableNetworking, "Disable all network activity")
	flag.StringVar(&c.Address, "address", c.Address,
		"IP Address to run application on. Leave empty to default to a public interface")
	flag.IntVar(&c.Port, "port", c.Port, "Port to run application on")
	flag.BoolVar(&c.WebInterface, "web-interface", c.WebInterface,
		"enable the web interface")
	flag.IntVar(&c.WebInterfacePort, "web-interface-port",
		c.WebInterfacePort, "port to serve web interface on")
	flag.StringVar(&c.WebInterfaceAddr, "web-interface-addr",
		c.WebInterfaceAddr, "addr to serve web interface on")
	flag.StringVar(&c.WebInterfaceCert, "web-interface-cert",
		c.WebInterfaceCert, "cert.pem file for web interface HTTPS. "+
			"If not provided, will use cert.pem in -data-directory")
	flag.StringVar(&c.WebInterfaceKey, "web-interface-key",
		c.WebInterfaceKey, "key.pem file for web interface HTTPS. "+
			"If not provided, will use key.pem in -data-directory")
	flag.BoolVar(&c.WebInterfaceHTTPS, "web-interface-https",
		c.WebInterfaceHTTPS, "enable HTTPS for web interface")

	flag.BoolVar(&c.RPCInterface, "rpc-interface", c.RPCInterface,
		"enable the rpc interface")
	flag.IntVar(&c.RPCInterfacePort, "rpc-interface-port", c.RPCInterfacePort,
		"port to serve rpc interface on")
	flag.StringVar(&c.RPCInterfaceAddr, "rpc-interface-addr", c.RPCInterfaceAddr,
		"addr to serve rpc interface on")

	flag.BoolVar(&c.LaunchBrowser, "launch-browser", c.LaunchBrowser,
		"launch system default webbrowser at client startup")
	flag.BoolVar(&c.PrintWebInterfaceAddress, "print-web-interface-address",
		c.PrintWebInterfaceAddress, "print configured web interface address and exit")
	flag.StringVar(&c.DataDirectory, "data-dir", c.DataDirectory,
		fmt.Sprintf("directory to store app data (defaults to ~/.%s)", coinName))
	flag.StringVar(&c.ConnectTo, "connect-to", c.ConnectTo,
		"connect to this ip only")
	flag.BoolVar(&c.ProfileCPU, "profile-cpu", c.ProfileCPU,
		"enable cpu profiling")
	flag.StringVar(&c.ProfileCPUFile, "profile-cpu-file",
		c.ProfileCPUFile, "where to write the cpu profile file")
	flag.BoolVar(&c.HTTPProf, "http-prof", c.HTTPProf,
		"Run the http profiling interface")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel,
		"Choices are: debug, info, notice, warning, error, critical")
	flag.BoolVar(&c.ColorLog, "color-log", c.ColorLog,
		"Add terminal colors to log output")
	flag.StringVar(&c.GUIDirectory, "gui-dir", c.GUIDirectory,
		"static content directory for the html gui")

	//Key Configuration Data
	flag.BoolVar(&c.RunMaster, "master", c.RunMaster,
		"run the daemon as blockchain master server")

	flag.StringVar(&BlockchainPubkeyStr, "master-public-key", BlockchainPubkeyStr,
		"public key of the master chain")
	flag.StringVar(&BlockchainSeckeyStr, "master-secret-key", BlockchainSeckeyStr,
		"secret key, set for master")

	flag.StringVar(&GenesisAddressStr, "genesis-address", GenesisAddressStr,
		"genesis address")
	flag.StringVar(&GenesisSignatureStr, "genesis-signature", GenesisSignatureStr,
		"genesis block signature")
	flag.Uint64Var(&c.GenesisTimestamp, "genesis-timestamp", c.GenesisTimestamp,
		"genesis block timestamp")

	flag.StringVar(&c.WalletDirectory, "wallet-dir", c.WalletDirectory,
		fmt.Sprintf("location of the wallet files. Defaults to ~/.%s/wallet/", coinName))

	flag.DurationVar(&c.OutgoingConnectionsRate, "connection-rate",
		c.OutgoingConnectionsRate, "How often to make an outgoing connection")
	flag.BoolVar(&c.LocalhostOnly, "localhost-only", c.LocalhostOnly,
		"Run on localhost and only connect to localhost peers")
	flag.BoolVar(&c.Arbitrating, "arbitrating", c.Arbitrating, "Run node in arbitrating mode")

	flag.UintVar(&c.RPCThreadNum, "rpc-thread-num", 5, "rpc thread number")
	flag.BoolVar(&c.Logtofile, "logtofile", false, "log to file")
}

var devConfig Config = Config{
	// Disable peer exchange
	DisablePEX: true,
	// Don't make any outgoing connections
	DisableOutgoingConnections: false,
	// Don't allowing incoming connections
	DisableIncomingConnections: false,
	// Disables networking altogether
	DisableNetworking: false,
	// Only run on localhost and only connect to others on localhost
	LocalhostOnly: false,
	// Which address to serve on. Leave blank to automatically assign to a
	// public interface
	Address: "",
	//gnet uses this for TCP incoming and outgoing
	Port: 7100,

	MaxConnections: 16,
	// How often to make outgoing connections, in seconds
	OutgoingConnectionsRate: time.Second * 5,
	// Wallet Address Version
	//AddressVersion: "test",
	// Remote web interface
	WebInterface:             true,
	WebInterfacePort:         7520,
	WebInterfaceAddr:         "127.0.0.1",
	WebInterfaceCert:         "",
	WebInterfaceKey:          "",
	WebInterfaceHTTPS:        false,
	PrintWebInterfaceAddress: false,

	RPCInterface:     true,
	RPCInterfacePort: 7530,
	RPCInterfaceAddr: "127.0.0.1",

	LaunchBrowser: true,
	// Data directory holds app data -- defaults to ~/.skycoin
	DataDirectory: fmt.Sprintf(".%s", coinName),
	// Web GUI static resources
	GUIDirectory: "./src/gui/static/",
	// Logging
	ColorLog: true,
	LogLevel: "DEBUG",

	// Wallets
	WalletDirectory: "",

	// Centralized network configuration
	RunMaster:        false,
	BlockchainPubkey: cipher.PubKey{},
	BlockchainSeckey: cipher.SecKey{},

	GenesisAddress:   cipher.Address{},
	GenesisTimestamp: GenesisTimestamp,
	GenesisSignature: cipher.Sig{},

	/* Developer options */

	// Enable cpu profiling
	ProfileCPU: false,
	// Where the file is written to
	ProfileCPUFile: fmt.Sprintf("%s.prof", coinName),
	// HTTP profiling interface (see http://golang.org/pkg/net/http/pprof/)
	HTTPProf: false,
	// Will force it to connect to this ip:port, instead of waiting for it
	// to show up as a peer
	ConnectTo: "",
}

func (c *Config) Parse() {
	c.register()
	flag.Parse()
	c.postProcess()
}

func (c *Config) postProcess() {
	var err error
	if GenesisSignatureStr != "" {
		c.GenesisSignature, err = cipher.SigFromHex(GenesisSignatureStr)
		panicIfError(err, "Invalid Signature")
	}
	if GenesisAddressStr != "" {
		c.GenesisAddress, err = cipher.DecodeBase58Address(GenesisAddressStr)
		panicIfError(err, "Invalid Address")
	}
	if BlockchainPubkeyStr != "" {
		c.BlockchainPubkey, err = cipher.PubKeyFromHex(BlockchainPubkeyStr)
		panicIfError(err, "Invalid Pubkey")
	}
	if BlockchainSeckeyStr != "" {
		c.BlockchainSeckey, err = cipher.SecKeyFromHex(BlockchainSeckeyStr)
		panicIfError(err, "Invalid Seckey")
		BlockchainSeckeyStr = ""
	}
	if BlockchainSeckeyStr != "" {
		c.BlockchainSeckey = cipher.SecKey{}
	}

	c.DataDirectory, err = file.InitDataDir(c.DataDirectory)
	panicIfError(err, "Invalid DataDirectory")

	if c.WebInterfaceCert == "" {
		c.WebInterfaceCert = filepath.Join(c.DataDirectory, "cert.pem")
	}
	if c.WebInterfaceKey == "" {
		c.WebInterfaceKey = filepath.Join(c.DataDirectory, "key.pem")
	}

	if c.WalletDirectory == "" {
		c.WalletDirectory = filepath.Join(c.DataDirectory, "wallets/")
	}

	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDirectory, "data.db")
	}
}

func panicIfError(err error, msg string, args ...interface{}) {
	if err != nil {
		log.Panicf(msg+": %v", append(args, err)...)
	}
}

func printProgramStatus() {
	fn := "goroutine.prof"
	logger.Debug("Writing goroutine profile to %s", fn)
	p := pprof.Lookup("goroutine")
	f, err := os.Create(fn)
	defer f.Close()
	if err != nil {
		logger.Error("%v", err)
		return
	}
	err = p.WriteTo(f, 2)
	if err != nil {
		logger.Error("%v", err)
		return
	}
}

func catchInterrupt(quit chan<- struct{}) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	close(quit)
}

// Catches SIGUSR1 and prints internal program state
func catchDebug() {
	sigchan := make(chan os.Signal, 1)
	//signal.Notify(sigchan, syscall.SIGUSR1)
	signal.Notify(sigchan, syscall.Signal(0xa)) // SIGUSR1 = Signal(0xa)
	for {
		select {
		case <-sigchan:
			printProgramStatus()
		}
	}
}

// init logging settings
func initLogging(dataDir string, level string, color, logtofile bool) (func(), error) {
	logCfg := logging.DevLogConfig(logModules)
	logCfg.Format = logFormat
	logCfg.Colors = color
	logCfg.Level = level

	var fd *os.File
	if logtofile {
		logDir := filepath.Join(dataDir, "logs")
		if err := createDirIfNotExist(logDir); err != nil {
			log.Println("initial logs folder failed", err)
			return nil, fmt.Errorf("init log folder fail, %v", err)
		}

		// open log file
		tf := "2006-01-02-030405"
		logfile := filepath.Join(logDir,
			fmt.Sprintf("%s-v%s.log", time.Now().Format(tf), Version))
		var err error
		fd, err = os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}

		logCfg.Output = io.MultiWriter(os.Stdout, fd)
	}

	logCfg.InitLogger()

	return func() {
		logger.Info("Log file closed")
		if fd != nil {
			fd.Close()
		}
	}, nil
}

func initProfiling(httpProf, profileCPU bool, profileCPUFile string) {
	if profileCPU {
		f, err := os.Create(profileCPUFile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if httpProf {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}
}

func configureDaemon(c *Config) daemon.Config {
	//cipher.SetAddressVersion(c.AddressVersion)

	dc := daemon.NewConfig()
	dc.Peers.DataDirectory = c.DataDirectory
	dc.Peers.Disabled = c.DisablePEX
	dc.Daemon.DisableOutgoingConnections = c.DisableOutgoingConnections
	dc.Daemon.DisableIncomingConnections = c.DisableIncomingConnections
	dc.Daemon.DisableNetworking = c.DisableNetworking
	dc.Daemon.Port = c.Port
	dc.Daemon.Address = c.Address
	dc.Daemon.LocalhostOnly = c.LocalhostOnly
	dc.Daemon.OutgoingMax = c.MaxConnections

	daemon.DefaultConnections = DefaultConnections

	if c.OutgoingConnectionsRate == 0 {
		c.OutgoingConnectionsRate = time.Millisecond
	}
	dc.Daemon.OutgoingRate = c.OutgoingConnectionsRate

	dc.Visor.Config.IsMaster = c.RunMaster

	dc.Visor.Config.BlockchainPubkey = c.BlockchainPubkey
	dc.Visor.Config.BlockchainSeckey = c.BlockchainSeckey

	dc.Visor.Config.GenesisAddress = c.GenesisAddress
	dc.Visor.Config.GenesisSignature = c.GenesisSignature
	dc.Visor.Config.GenesisTimestamp = c.GenesisTimestamp
	dc.Visor.Config.GenesisCoinVolume = GenesisCoinVolume
	dc.Visor.Config.DBPath = c.DBPath
	dc.Visor.Config.Arbitrating = c.Arbitrating
	return dc
}

// Run starts the shellcoin node
func Run(c *Config) {
	defer func() {
		// try catch panic in main thread
		if r := recover(); r != nil {
			logger.Error("recover: %v\nstack:%v", r, string(debug.Stack()))
		}
	}()

	c.GUIDirectory = file.ResolveResourceDirectory(c.GUIDirectory)

	scheme := "http"
	if c.WebInterfaceHTTPS {
		scheme = "https"
	}
	host := fmt.Sprintf("%s:%d", c.WebInterfaceAddr, c.WebInterfacePort)
	fullAddress := fmt.Sprintf("%s://%s", scheme, host)
	logger.Critical("Full address: %s", fullAddress)

	if c.PrintWebInterfaceAddress {
		fmt.Println(fullAddress)
		return
	}

	initProfiling(c.HTTPProf, c.ProfileCPU, c.ProfileCPUFile)

	closelog, err := initLogging(c.DataDirectory, c.LogLevel, c.ColorLog, c.Logtofile)
	if err != nil {
		fmt.Println(err)
		return
	}

	// If the user Ctrl-C's, shutdown properly
	quit := make(chan struct{})

	go catchInterrupt(quit)
	// Watch for SIGUSR1
	go catchDebug()

	gui.InitWalletRPC(c.WalletDirectory, wallet.OptCoin("sc2"))

	dconf := configureDaemon(c)
	d, err := daemon.NewDaemon(dconf)
	if err != nil {
		logger.Error("%v", err)
		return
	}

	errC := make(chan error, 1)

	go func() {
		errC <- d.Run()
	}()

	var rpc *webrpc.WebRPC
	// start the webrpc
	if c.RPCInterface {
		rpc, err = webrpc.New(
			fmt.Sprintf("%v:%v", c.RPCInterfaceAddr, c.RPCInterfacePort),
			webrpc.ChanBuffSize(1000),
			webrpc.ThreadNum(c.RPCThreadNum),
			webrpc.Gateway(d.Gateway))
		if err != nil {
			logger.Error("%v", err)
			return
		}

		go func() {
			errC <- rpc.Run()
		}()
	}

	// Debug only - forces connection on start.  Violates thread safety.
	if c.ConnectTo != "" {
		if err := d.Pool.Pool.Connect(c.ConnectTo); err != nil {
			logger.Error("Force connect %s failed, %v", c.ConnectTo, err)
			return
		}
	}

	if c.WebInterface {
		var err error
		if c.WebInterfaceHTTPS {
			// Verify cert/key parameters, and if neither exist, create them
			errs := cert.CreateCertIfNotExists(host, c.WebInterfaceCert, c.WebInterfaceKey, "Shellcoind")
			if len(errs) != 0 {
				for _, err := range errs {
					logger.Error(err.Error())
				}
				logger.Error("gui.CreateCertIfNotExists failure")
				return
			}

			err = gui.LaunchWebInterfaceHTTPS(host, c.GUIDirectory, d, c.WebInterfaceCert, c.WebInterfaceKey)
		} else {
			err = gui.LaunchWebInterface(host, c.GUIDirectory, d)
		}

		if err != nil {
			logger.Error(err.Error())
			logger.Error("Failed to start web GUI")
			return
		}

		if c.LaunchBrowser {
			go func() {
				// Wait a moment just to make sure the http interface is up
				time.Sleep(time.Millisecond * 100)

				logger.Info("Launching System Browser with %s", fullAddress)
				if err := browser.Open(fullAddress); err != nil {
					logger.Error(err.Error())
					return
				}
			}()
		}
	}

	// if d.Visor.HeadBkSeq() < 2 {
	// 	time.Sleep(5)
	// 	tx := InitTransaction()
	// 	_ = tx
	// 	_, err := d.Visor.InjectTransaction(tx, d.Pool)
	// 	if err != nil {
	// 		//	log.Panic(err)
	// 	}
	// }

	/*
		//first transaction
		if c.RunMaster == true {
			go func() {
				for d.Visor.Visor.Blockchain.Head().Seq() < 2 {
					time.Sleep(5)
					tx := InitTransaction()
					err, _ := d.Visor.Visor.InjectTxn(tx)
					if err != nil {
						//log.Panic(err)
					}
				}
			}()
		}
	*/

	select {
	case <-quit:
	case err := <-errC:
		logger.Error("%v", err)
	}

	logger.Info("Shutting down...")

	if rpc != nil {
		rpc.Shutdown()
	}

	gui.Shutdown()
	d.Shutdown()
	closelog()
	logger.Info("Goodbye")
}

func main() {
	devConfig.Parse()
	Run(&devConfig)
}

//addresses for storage of coins
var AddrList = []string{
	"Z1k6qej1yPoNAZmRVCGQW8t5zyW2b8EYSa",
	"2BWPusJggEF8zHTdQyy62oTAre8m326kNTG",
	"2abzMYGi6HFP8F8ZyUcdWY1fXTvgncLxxUg",
	"2cP8ed8C7ugK2BozeVnVhVfFQ1HL1tixsPq",
	"yQ4FP8iRjN7fwLwi3RrJZaNJf63LNoijKo",
	"2epqomKZVWGxaDpaZkwu9QHM9RaFsAHgziQ",
	"SLKXabjERNatNZfrWQDzVvxJt2SSYMx7DN",
	"xYS16WXoa7qoN6CCibmPtW938F7JiNYJmo",
	"9u1jDCeK7WSVs1wuoQYK3GD3PqUGtTNLUz",
	"2FbNyvNqMzEvrRrv4kcsp4GxLZQPWHzUwru",
	"2Z5dMNPe9Dd5FPTWWwTsRfREwBf53wftz3G",
	"2ZpN1NMvoM3brt1LiQbxSUT1nK14tiTKmjs",
	"qGFwwYHJtYSH26Bg3E33Zi7EQXMj45ECU7",
	"2Y6gK4992vHPJJ1cUQa7GmkHTophhqkfqNp",
	"2Ur7H3NJ6uivPx4W1uuDhfeu92E7tXFgbbk",
	"yX78JsBww6qRM9Cy6r1cMcM1cqqfvLB81G",
	"2Wu7pUcSQVXBMF6iHT5GK1JRHToYc5VwEMU",
	"sX8V7KE5XDgtFsxUTF8nJoXCAS4WG6qmdL",
	"BMzo8uGGtTswKCLiUGQYQEC3TSgFUYxGUV",
	"WYUGd1PJtFkjAtzFULcYcXwzM4RwD4Bohg",
	"8D3QqVJ1hLhmwK9dH6CjK8j8PmzBZdFXKi",
	"zZLbkWDxiZPvziLXiT8Bgir1bP94dGkSv8",
	"cDAzgCAxKphwdhWB5ziSxrBzS8J3FxKrVX",
	"W64hnPJ8h48CErFoAJDunC7FDTXCiDBYaU",
	"rkTX8iDP4FbZHkomBze6UkHDmhvw54vrci",
	"2i4iBDtiagR3jiegnmbiVZ1tkTp6vwVFjXF",
	"wFCxPvzVBce1APxbFGgpPFnTN5cQUnLqEp",
	"KLW1sJUTuGQcQATaMrkMeEzMhKqFE9SzrM",
	"2zxjynmyjHwyA9CmtEpnKBUc2t3k7L7YxT",
	"ZpcAH9SdsUfgPgJUt7kY5FdvSkxMAy7AmW",
	"XV7YcAQujembG8pWksdjqraWfoGdpxUcfy",
	"Exskppefm5J35ejwS7HQTRUtCKNacwbENc",
	"Y7ngaBfePwGy4vpNgyxXcw6EdHkuNkLdh",
	"2VW4y4MjEceMtkHkoPPC5DCVATZJJ4wFd61",
	"2jF6FVDs3trWCg2AjdTGBbJbqBL1zo83aXV",
	"eAdXVjvfwATdMsQNxWCwn9fmrjj4yV1gYk",
	"JnuJTVDw2SBh559G4TKVdDSMZXgizBgAWX",
	"2XjofbRTnCU5F5PFMErLmivfveCJj1PyyKX",
	"jY7D2siBSdtCnW2qZQxtAHNMiVtzp72dpy",
	"Pqho6tzNGn1oCjx67oJ64FjZsodcbCfZA8",
	"2LxCTzJFfNCxDvkhq8cgfq4P6T2bK5qVxKh",
	"Wf2ZE5NswFjSiMc4beFG3kG5jppMU77NA",
	"7ThrKqp68NLgSCAvFq75CghNDCVzPEV8pb",
	"2FXWH1yNkrpSm99uggshpAxyRxu4BDsLp43",
	"2VcS5bRbATZRfyfGqxtdcVsCMnFcU5YRC5w",
	"cAyxv5CRpUWqNZesjU4dsw9zsgp3wo1mzC",
	"uxRrW927Thp9kBUqCCF5uGizQNYN9GJZBb",
	"usSvw63y1HrHxYpksjBhcnrFSDQao619KN",
	"urVo1mKQa4wZR79ViFeDfsBA1hmNRz4hgA",
	"21wFQjVV8HvWtKcyA9gzb8rQeFLYUFRE5S1",
	"2C8mNpRfEsAfjMYqc8DwqdzYz1XsWdX8Lpg",
	"2SVd9pGm59rafUAc3i5fWUwiEvc9QDz7Cqd",
	"ZRvUwGFJ58y5H9rwUEqJNdPEr4DiAjRgHm",
	"H7a1oj4KvWKkGntnHh2uZoGyrvM3pJgGze",
	"4YffwkMK5Fmxc5KcXKaQs1QJU9yLHijrrx",
	"2TkyDHnoGJcLbdsNYFUwccm113GF8MMCETj",
	"2CFTZWhHDEXwX1N1NRwNZ9bEErzw82SKNyx",
	"Nxrs7pRYWGbxEByhZk14qJoJYbUuPgF1LD",
	"2GbczQuUZe3CDt6fQEkmmYxruTZ2JW79uAs",
	"2UoAuTGkA8xq6bh9Hs6KXNTLkD459THrxiy",
	"q6w4yZbzLoNWmfq5MopSwPkzPJqrroWq8v",
	"Ee6bq2Gmxp8m5aCX4jvG1yBPawLvTttGNX",
	"2MSTLjxJm2oPLzQCYrbm1zkFR5xEnQeFi3h",
	"27WRJ87b3DxEvmh8axkgEews8DYycjN91xm",
	"29b2GCEQrM7vF7pngHKtvVrTXCL1DJU3s9Y",
	"2NrKPH7UH3jhhEW7JVsExLpTtJQ6kYfcmw1",
	"SWPF7b9d6KPn3XXiq6tdkoLcWTiPpTD7Ke",
	"2eQf3cgtUVSpNScJjwNgoEyHuVXjao6GXw3",
	"2Ltm64ZjNcaP85jNUgucpKNHZDcrnFmiyEk",
	"WuRJen3KavvZxyKVaPHcUvJW6CtWT33iRp",
	"pf4g5BXa3Scq5yRiX7redAP6c3up6BrysT",
	"aGx9x1qMqNyxDVwD5PY7hQ1RaYCuTX6GX5",
	"2hU1xk3Zy4NrfAKuB28ZtEbQNPum6pjjZ2b",
	"21p72H7FJ1g3jMM178ozj1BjzsAnkKskYBm",
	"5f3Rh5dPHGoV1z1kbWNApJVreQUAKaJiXz",
	"wZJjmL184D1ZyfmHmfrZX8UKH6RyEYFMJ8",
	"BxvJsGCV1n2G7PZEEE3ZQuUeSNAfszEcWB",
	"2YUhuUVPJERRCo5d8KMZFRXE5XZdmHus547",
	"tUq54Hdd4Pyp5vEXN7zC5fNk1UjyomgTae",
	"A2vjGTqvXyTaopoUhiNiNyJKoRrXPRobSm",
	"npfUWxQg42wWNXwYhUPAS76cDgVtm44H4n",
	"gYuosHMTDoYTEgmUkYoDppZy1PjJRW5eyB",
	"J45zxYi9UUUvCK5Pv51gHPH1a33FuPL2LJ",
	"2LDfyyL9EL9gSe3aC5rM8EpMh4zn3BJWfk",
	"2cH1VBMKSWW6WftBs7CGwLrWESTkxhc2xMX",
	"tmEmVBTDxTpbGNwMdBJLTWrLn4VnpFphmZ",
	"DjemT2f8iAwk7MnxMcpjy75urf1Vz5rgsV",
	"2RRa36ELR8jDm8m5Mf6DS3tskMw1PknCxAz",
	"cEKEgZyjjMR2mmnfFHgmFjAvVwmzeowiYg",
	"A2ohw5ZgMHdsJpskEgd4pQny3DtH54Sfqi",
	"2bFQnRXmFr3coS7v4tJh6AC4mfUJTTRVQME",
	"PLsUxXTJJWKuZVnMxP3FYbnMk1DBxXyBoJ",
	"n5fMHy77JrfQawvZ4SKfsiVkfvuN1YnCyQ",
	"2f2Uu4BbbcWMX4qe1uFm8UgPsZkHsCB7t5U",
	"W1SQKqzBDxVyVkkjTx1aEFFC1vDaf6cujg",
	"Yf9D1EHKL5tjAiNjsVTcon5aARz7orb4mo",
	"28eVNsyvCYx85qgwF9FYJLseKyoknDVig21",
	"pYzWzz1oxFbdtg2UrXmebNvhmzhGbBt9Q",
	"afAj8nZ92jBMnMNcDKyHSkAyBEHf65tW6n",
	"2mB9UQbsiB4jFooBLYgnBk3hB8GnEEYVadz",
}

func InitTransaction() coin.Transaction {
	var tx coin.Transaction

	output := cipher.MustSHA256FromHex("00406e0f9f553e690b5c59bdf602391134fd1a853f004e56c7c1198ef25e1f6d")
	tx.PushInput(output)

	for i := 0; i < 100; i++ {
		addr := cipher.MustDecodeBase58Address(AddrList[i])
		tx.PushOutput(addr, 3e12, 1) // 10e6*10e6
	}

	if false {
		seckeys := make([]cipher.SecKey, 1)
		seckey := ""
		seckeys[0] = cipher.MustSecKeyFromHex(seckey)
		tx.SignInputs(seckeys)
	} else {
		txs := make([]cipher.Sig, 1)
		sig := "62666c6a4a431e13db2f9aab33eb798e53443bc4d264b72ed880c62a9acd108773f15d44e6d9df329c88a6dbdf7d73480b5f76acf2aa864a8185e92cc6dc096c01"
		txs[0] = cipher.MustSigFromHex(sig)
		tx.Sigs = txs
	}
	tx.UpdateHeader()

	err := tx.Verify()

	if err != nil {
		log.Panic(err)
	}

	log.Printf("init tx signature= %s", tx.Sigs[0].Hex())
	return tx
}

func createDirIfNotExist(dir string) error {
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return nil
	}

	return os.Mkdir(dir, 0777)
}
