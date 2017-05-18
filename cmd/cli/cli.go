package main

import (
	"fmt"
	"os"

	"strings"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/util"
)

func main() {
	// get rpc address from env
	rpcAddr := os.Getenv("RPC_ADDR")
	if rpcAddr == "" {
		rpcAddr = "127.0.0.1:7530"
	}

	// get wallet dir from env
	wltDir := os.Getenv("WALLET_DIR")
	if wltDir == "" {
		home := util.UserHome()
		wltDir = home + "/.shellcoin/wallets"
	}

	// get wallet name from env
	wltName := os.Getenv("WALLET_NAME")
	if wltName == "" {
		wltName = "shellcoin_cli.wlt"
	} else {
		if !strings.HasSuffix(wltName, ".wlt") {
			fmt.Println("value of 'WALLET_NAME' env is not correct, must has .wlt extenstion")
			return
		}
	}

	// init the cli
	if err := cli.NewApp(cli.RPCAddr(rpcAddr),
		cli.WalletDir(wltDir),
		cli.DefaultWltName(wltName),
		cli.Coin("shellcoin2.0")).Run(os.Args); err != nil {
		fmt.Println(err)
	}
}
