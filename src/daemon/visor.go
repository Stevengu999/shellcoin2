package daemon

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	//"github.com/skycoin/skycoin/src/daemon/gnet"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/daemon/gnet"
	"github.com/skycoin/skycoin/src/util"
	"github.com/skycoin/skycoin/src/visor"
	//"github.com/skycoin/skycoin/src/wallet"
)

//TODO
//- download block headers
//- request blocks individually across multiple peers

//TODO
//- use CXO for blocksync

/*
Visor should not be duplicated
- this should be pushed into /src/visor
*/

type VisorConfig struct {
	Config visor.VisorConfig
	// Disabled the visor completely
	Disabled bool
	// How often to request blocks from peers
	BlocksRequestRate time.Duration
	// How often to announce our blocks to peers
	BlocksAnnounceRate time.Duration
	// How many blocks to respond with to a GetBlocksMessage
	BlocksResponseCount uint64
	//how long between saving copies of the blockchain
	BlockchainBackupRate time.Duration
}

func NewVisorConfig() VisorConfig {
	return VisorConfig{
		Config:               visor.NewVisorConfig(),
		Disabled:             false,
		BlocksRequestRate:    time.Second * 60, //backup, could be disabled
		BlocksAnnounceRate:   time.Second * 60, //backup, could be disabled
		BlocksResponseCount:  20,
		BlockchainBackupRate: time.Second * 30,
	}
}

type Visor struct {
	Config VisorConfig
	v      *visor.Visor
	// Peer-reported blockchain length.  Use to estimate download progress
	blockchainLengths map[string]uint64
	reqC              chan reqFunc // all request will go through this channel, to keep writing and reading member variable thread safe.
	cancel            context.CancelFunc
}

type reqFunc func(context.Context)

// NewVisor creates visor instance
func NewVisor(c VisorConfig) *Visor {
	var v *visor.Visor
	if !c.Disabled {
		v = visor.NewVisor(c.Config)
	}

	vs := &Visor{
		Config:            c,
		v:                 v,
		blockchainLengths: make(map[string]uint64),
		reqC:              make(chan reqFunc, 100),
	}

	cxt, cancel := context.WithCancel(context.Background())
	vs.cancel = cancel
	go vs.run(cxt)
	return vs
}

func (vs *Visor) run(cxt context.Context) {
	for {
		select {
		case <-cxt.Done():
			return
		case req := <-vs.reqC:
			req(cxt)
		}
	}
}

// the callback function must not be blocked.
func (vs *Visor) strand(f func()) {
	done := make(chan struct{})
	vs.reqC <- func(cxt context.Context) {
		defer close(done)
		c := make(chan struct{})
		go func() {
			defer close(c)
			f()
		}()
		select {
		case <-cxt.Done():
			return
		case <-c:
			return
		}
	}
	<-done
}

// Shutdown closes the visor
func (vs *Visor) Shutdown() {
	vs.cancel()
}

// RefreshUnconfirmed checks unconfirmed txns against the blockchain and purges ones too old
func (vs *Visor) RefreshUnconfirmed() {
	if vs.Config.Disabled {
		return
	}
	vs.strand(func() {
		vs.v.RefreshUnconfirmed()
	})
}

// RequestBlocks Sends a GetBlocksMessage to all connections
func (vs *Visor) RequestBlocks(pool *Pool) {
	if vs.Config.Disabled {
		return
	}
	vs.strand(func() {
		m := NewGetBlocksMessage(vs.v.HeadBkSeq(), vs.Config.BlocksResponseCount)
		pool.Pool.BroadcastMessage(m)
	})
}

// AnnounceBlocks sends an AnnounceBlocksMessage to all connections
func (vs *Visor) AnnounceBlocks(pool *Pool) {
	if vs.Config.Disabled {
		return
	}
	vs.strand(func() {
		m := NewAnnounceBlocksMessage(vs.v.HeadBkSeq())
		pool.Pool.BroadcastMessage(m)
	})
}

// RequestBlocksFromAddr sends a GetBlocksMessage to one connected address
func (vs *Visor) RequestBlocksFromAddr(pool *Pool, addr string) error {
	if vs.Config.Disabled {
		return errors.New("Visor disabled")
	}
	var err error
	vs.strand(func() {
		m := NewGetBlocksMessage(vs.v.HeadBkSeq(), vs.Config.BlocksResponseCount)
		if !pool.Pool.IsConnExist(addr) {
			err = fmt.Errorf("Tried to send GetBlocksMessage to %s, but we're "+
				"not connected", addr)
			return
		}
		pool.Pool.SendMessage(addr, m)
	})
	return err
}

// SetTxnsAnnounced sets all txns as announced
func (vs *Visor) SetTxnsAnnounced(txns []cipher.SHA256) {
	vs.strand(func() {
		now := util.Now()
		for _, h := range txns {
			vs.v.Unconfirmed.SetAnnounced(h, now)
		}
	})
}

// Sends a signed block to all connections.
// TODO: deprecate, should only send to clients that request by hash
func (vs *Visor) broadcastBlock(sb coin.SignedBlock, pool *Pool) {
	if vs.Config.Disabled {
		return
	}
	m := NewGiveBlocksMessage([]coin.SignedBlock{sb})
	pool.Pool.BroadcastMessage(m)
}

// BroadcastTransaction broadcasts a single transaction to all peers.
func (vs *Visor) BroadcastTransaction(t coin.Transaction, pool *Pool) {
	if vs.Config.Disabled {
		logger.Debug("broadcast tx disabled")
		return
	}
	m := NewGiveTxnsMessage(coin.Transactions{t})
	logger.Debug("Broadcasting GiveTxnsMessage to %d conns", pool.Pool.Size())
	pool.Pool.BroadcastMessage(m)
}

//move into visor
//DEPRECATE
func (vs *Visor) InjectTransaction(txn coin.Transaction, pool *Pool) (coin.Transaction, error) {
	var err error
	vs.strand(func() {
		err = visor.VerifyTransactionFee(vs.v.Blockchain, &txn)
		if err != nil {
			return
		}

		err = txn.Verify()
		if err != nil {
			err = fmt.Errorf("Transaction Verification Failed, %v", err)
			return
		}

		err, _ := vs.v.InjectTxn(txn)
		if err != nil {
			return
		}
		vs.BroadcastTransaction(txn, pool)
	})
	return txn, err
}

// ResendTransaction resends a known UnconfirmedTxn.
func (vs *Visor) ResendTransaction(h cipher.SHA256, pool *Pool) {
	if vs.Config.Disabled {
		return
	}
	vs.strand(func() {
		if ut, ok := vs.v.Unconfirmed.Txns[h]; ok {
			vs.BroadcastTransaction(ut.Txn, pool)
		}
	})
	return
}

// ResendUnconfirmedTxns resents all unconfirmed transactions
func (vs *Visor) ResendUnconfirmedTxns(pool *Pool) []cipher.SHA256 {
	var txids []cipher.SHA256
	if vs.Config.Disabled {
		return txids
	}
	vs.strand(func() {
		var txns []visor.UnconfirmedTxn
		for _, unconfirmTxn := range vs.v.Unconfirmed.Txns {
			txns = append(txns, unconfirmTxn)
		}

		// sort the txns by receive time
		sort.Sort(byTxnRecvTime(txns))

		for i := range txns {
			vs.BroadcastTransaction(txns[i].Txn, pool)
			txids = append(txids, txns[i].Txn.Hash())
		}
	})
	return txids
}

// CreateAndPublishBlock creates a block from unconfirmed transactions and sends it to the network.
// Will panic if not running as a master chain.  Returns creation error and
// whether it was published or not
func (vs *Visor) CreateAndPublishBlock(pool *Pool) error {
	if vs.Config.Disabled {
		return errors.New("Visor disabled")
	}
	var err error
	vs.strand(func() {
		var sb coin.SignedBlock
		sb, err = vs.v.CreateAndExecuteBlock()
		if err != nil {
			return
		}
		vs.broadcastBlock(sb, pool)
	})
	return err
}

// RemoveConnection updates internal state when a connection disconnects
func (vs *Visor) RemoveConnection(addr string) {
	vs.strand(func() {
		delete(vs.blockchainLengths, addr)
	})
}

// RecordBlockchainLength saves a peer-reported blockchain length
func (vs *Visor) RecordBlockchainLength(addr string, bkLen uint64) {
	vs.strand(func() {
		vs.blockchainLengths[addr] = bkLen
	})
}

// EstimateBlockchainLength returns the blockchain length estimated from peer reports
// Deprecate. Should not need. Just report time of last block
func (vs *Visor) EstimateBlockchainLength() uint64 {
	var l uint64
	vs.strand(func() {
		ourLen := vs.v.HeadBkSeq() + 1
		if len(vs.blockchainLengths) < 2 {
			l = ourLen
			return
		}
		lengths := make(BlockchainLengths, len(vs.blockchainLengths))
		i := 0
		for _, seq := range vs.blockchainLengths {
			lengths[i] = seq
			i++
		}
		sort.Sort(lengths)
		median := len(lengths) / 2
		var val uint64
		if len(lengths)%2 == 0 {
			val = (lengths[median] + lengths[median-1]) / 2
		} else {
			val = lengths[median]
		}

		if val >= l {
			l = val
		}
	})
	return l
}

// HeadBkSeq returns the head sequence
func (vs *Visor) HeadBkSeq() uint64 {
	var seq uint64
	vs.strand(func() {
		seq = vs.v.HeadBkSeq()
	})
	return seq
}

// ExecuteSignedBlock executes signed block
func (vs *Visor) ExecuteSignedBlock(b coin.SignedBlock) error {
	var err error
	vs.strand(func() {
		err = vs.v.ExecuteSignedBlock(b)
	})
	return err
}

// GetSignedBlocksSince returns numbers of signed blocks since seq.
func (vs *Visor) GetSignedBlocksSince(seq uint64, num uint64) []coin.SignedBlock {
	var sbs []coin.SignedBlock
	vs.strand(func() {
		sbs = vs.v.GetSignedBlocksSince(seq, num)
	})
	return sbs
}

func (vs *Visor) UnConfirmFilterKnown(txns []cipher.SHA256) []cipher.SHA256 {
	var ts []cipher.SHA256
	vs.strand(func() {
		ts = vs.v.Unconfirmed.FilterKnown(txns)
	})
	return ts
}

func (vs *Visor) UnConfirmKnow(hashes []cipher.SHA256) (txns coin.Transactions) {
	vs.strand(func() {
		txns = vs.v.Unconfirmed.GetKnown(hashes)
	})
	return
}

// InjectTxn only try to append transaction into local blockchain, don't broadcast it.
func (vs *Visor) InjectTxn(tx coin.Transaction) (err error, know bool) {
	vs.strand(func() {
		err, know = vs.v.InjectTxn(tx)
	})
	return
}

// Communication layer for the coin pkg

// Sent to request blocks since LastBlock
type GetBlocksMessage struct {
	LastBlock       uint64
	RequestedBlocks uint64
	c               *gnet.MessageContext `enc:"-"`
}

func NewGetBlocksMessage(lastBlock uint64, requestedBlocks uint64) *GetBlocksMessage {
	return &GetBlocksMessage{
		LastBlock:       lastBlock,
		RequestedBlocks: requestedBlocks, //count of blocks requested
	}
}

func (self *GetBlocksMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

/*
	Should send number to be requested, with request
*/
func (self *GetBlocksMessage) Process(d *Daemon) {
	// TODO -- we need the sig to be sent with the block, but only the master
	// can sign blocks.  Thus the sig needs to be stored with the block.
	// TODO -- move 20 to either Messages.Config or Visor.Config
	if d.Visor.Config.Disabled {
		return
	}
	// Record this as this peer's highest block
	d.Visor.RecordBlockchainLength(self.c.Addr, self.LastBlock)
	// Fetch and return signed blocks since LastBlock
	//blocks := d.Visor.Visor.GetSignedBlocksSince(self.LastBlock,
	//	d.Visor.Config.BlocksResponseCount)
	blocks := d.Visor.GetSignedBlocksSince(self.LastBlock,
		self.RequestedBlocks)
	logger.Debug("Got %d blocks since %d", len(blocks), self.LastBlock)
	if len(blocks) == 0 {
		return
	}
	m := NewGiveBlocksMessage(blocks)
	d.Pool.Pool.SendMessage(self.c.Addr, m)
}

// Sent in response to GetBlocksMessage, or unsolicited
type GiveBlocksMessage struct {
	Blocks []coin.SignedBlock
	c      *gnet.MessageContext `enc:"-"`
}

func NewGiveBlocksMessage(blocks []coin.SignedBlock) *GiveBlocksMessage {
	return &GiveBlocksMessage{
		Blocks: blocks,
	}
}

func (self *GiveBlocksMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

func (self *GiveBlocksMessage) Process(d *Daemon) {
	if d.Visor.Config.Disabled {
		logger.Critical("Visor disabled, ignoring GiveBlocksMessage")
		return
	}
	processed := 0
	maxSeq := d.Visor.HeadBkSeq()
	for _, b := range self.Blocks {
		// To minimize waste when receiving multiple responses from peers
		// we only break out of the loop if the block itself is invalid.
		// E.g. if we request 20 blocks since 0 from 2 peers, and one peer
		// replies with 15 and the other 20, if we did not do this check and
		// the reply with 15 was received first, we would toss the one with 20
		// even though we could process it at the time.
		if b.Block.Head.BkSeq <= maxSeq {
			continue
		}
		err := d.Visor.ExecuteSignedBlock(b)
		if err == nil {
			logger.Critical("Added new block %d", b.Block.Head.BkSeq)
			processed++
		} else {
			logger.Critical("Failed to execute received block: %v", err)
			// Blocks must be received in order, so if one fails its assumed
			// the rest are failing
			break
		}
	}
	logger.Critical("Processed %d/%d blocks", processed, len(self.Blocks))
	if processed == 0 {
		return
	}
	// Announce our new blocks to peers
	m1 := NewAnnounceBlocksMessage(d.Visor.HeadBkSeq())
	d.Pool.Pool.BroadcastMessage(m1)
	//request more blocks.
	m2 := NewGetBlocksMessage(d.Visor.HeadBkSeq(), d.Visor.Config.BlocksResponseCount)
	d.Pool.Pool.BroadcastMessage(m2)
}

// Tells a peer our highest known BkSeq. The receiving peer can choose
// to send GetBlocksMessage in response
type AnnounceBlocksMessage struct {
	MaxBkSeq uint64
	c        *gnet.MessageContext `enc:"-"`
}

func NewAnnounceBlocksMessage(seq uint64) *AnnounceBlocksMessage {
	return &AnnounceBlocksMessage{
		MaxBkSeq: seq,
	}
}

func (self *AnnounceBlocksMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

func (self *AnnounceBlocksMessage) Process(d *Daemon) {
	if d.Visor.Config.Disabled {
		return
	}
	headBkSeq := d.Visor.HeadBkSeq()
	if headBkSeq >= self.MaxBkSeq {
		return
	}
	//should this be block get request for current sequence?
	//if client is not caught up, wont attempt to get block
	m := NewGetBlocksMessage(headBkSeq, d.Visor.Config.BlocksResponseCount)
	d.Pool.Pool.SendMessage(self.c.Addr, m)
}

type SendingTxnsMessage interface {
	GetTxns() []cipher.SHA256
}

// Tells a peer that we have these transactions
type AnnounceTxnsMessage struct {
	Txns []cipher.SHA256
	c    *gnet.MessageContext `enc:"-"`
}

func NewAnnounceTxnsMessage(txns []cipher.SHA256) *AnnounceTxnsMessage {
	return &AnnounceTxnsMessage{
		Txns: txns,
	}
}

func (self *AnnounceTxnsMessage) GetTxns() []cipher.SHA256 {
	return self.Txns
}

func (self *AnnounceTxnsMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

func (self *AnnounceTxnsMessage) Process(d *Daemon) {
	if d.Visor.Config.Disabled {
		return
	}
	unknown := d.Visor.UnConfirmFilterKnown(self.Txns)
	if len(unknown) == 0 {
		return
	}
	m := NewGetTxnsMessage(unknown)
	d.Pool.Pool.SendMessage(self.c.Addr, m)
}

type GetTxnsMessage struct {
	Txns []cipher.SHA256
	c    *gnet.MessageContext `enc:"-"`
}

func NewGetTxnsMessage(txns []cipher.SHA256) *GetTxnsMessage {
	return &GetTxnsMessage{
		Txns: txns,
	}
}

func (self *GetTxnsMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

func (self *GetTxnsMessage) Process(d *Daemon) {
	if d.Visor.Config.Disabled {
		return
	}
	// Locate all txns from the unconfirmed pool
	// reply to sender with GiveTxnsMessage
	known := d.Visor.UnConfirmKnow(self.Txns)
	if len(known) == 0 {
		return
	}
	logger.Debug("%d/%d txns known", len(known), len(self.Txns))
	m := NewGiveTxnsMessage(known)
	d.Pool.Pool.SendMessage(self.c.Addr, m)
}

type GiveTxnsMessage struct {
	Txns coin.Transactions
	c    *gnet.MessageContext `enc:"-"`
}

func NewGiveTxnsMessage(txns coin.Transactions) *GiveTxnsMessage {
	return &GiveTxnsMessage{
		Txns: txns,
	}
}

func (self *GiveTxnsMessage) GetTxns() []cipher.SHA256 {
	return self.Txns.Hashes()
}

func (self *GiveTxnsMessage) Handle(mc *gnet.MessageContext,
	daemon interface{}) error {
	self.c = mc
	return daemon.(*Daemon).recordMessageEvent(self, mc)
}

func (self *GiveTxnsMessage) Process(d *Daemon) {
	if d.Visor.Config.Disabled {
		return
	}

	if len(self.Txns) > 32 {
		logger.Warning("More than 32 transactions in pool. Implement breaking transactions transmission into multiple packets")
	}

	hashes := make([]cipher.SHA256, 0, len(self.Txns))
	// Update unconfirmed pool with these transactions
	for _, txn := range self.Txns {
		// Only announce transactions that are new to us, so that peers can't
		// spam relays
		if err, known := d.Visor.InjectTxn(txn); err == nil && !known {
			hashes = append(hashes, txn.Hash())
		} else {
			if !known {
				logger.Warning("Failed to record txn: %v", err)
			} else {
				logger.Warning("Duplicate Transation: ")
			}
		}
	}
	// Announce these transactions to peers
	if len(hashes) != 0 {
		m := NewAnnounceTxnsMessage(hashes)
		d.Pool.Pool.BroadcastMessage(m)
	}
}

type BlockchainLengths []uint64

func (self BlockchainLengths) Len() int {
	return len(self)
}

func (self BlockchainLengths) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

func (self BlockchainLengths) Less(i, j int) bool {
	return self[i] < self[j]
}

type byTxnRecvTime []visor.UnconfirmedTxn

func (txs byTxnRecvTime) Len() int {
	return len(txs)
}

func (txs byTxnRecvTime) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}

func (txs byTxnRecvTime) Less(i, j int) bool {
	return txs[i].Received.Nanosecond() < txs[j].Received.Nanosecond()
}
