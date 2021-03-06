// This is the network client for communication with nodes
// It is used by nodes to communicate with other nodes and by lite wallets
// to communicate with nodes
package nodeclient

import (
	"bytes"
	"encoding/binary"
	"time"

	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"

	netlib "github.com/gelembjuk/oursql/lib/net"
	"github.com/gelembjuk/oursql/lib/utils"
)

const (
	CommandAddresses        = "addr"
	CommandGetConsensusData = "getcnsdata"
	CommandGetFirstBlocks   = "getfblocks"
	CommandGetBalance       = "getbalance"
	CommandGetState         = "getstate"
	CommandGetUpdates       = "getupdates"
	CommandGetTransaction   = "gettransact"
	CommandCheckBlock       = "checkblock"
	CommandGetBlock         = "getblock" // requests a block by hash
	CommandBlock            = "block"    // send block body

)

type NodeClient struct {
	DataDir     string
	NodeAddress netlib.NodeAddr
	Address     string // wallet address
	Logger      *utils.LoggerMan
	NodeNet     *netlib.NodeNetwork
	NodeAuthStr string
}

// Command to send list of known addresses to other node
type ComAddresses struct {
	AddrFrom  netlib.NodeAddr
	Addresses []netlib.NodeAddr
}

type ComBlock struct {
	AddrFrom netlib.NodeAddr
	Block    []byte
}

// this struct can be used for 2 commands. to get blocks starting from some block to down or to up
type ComGetBlocks struct {
	AddrFrom  netlib.NodeAddr
	StartFrom []byte // has of block from which to start and go down or go up in case of Up command
}

// Response of GetBlock request
type ComGetFirstBlocksData struct {
	Blocks [][]byte // lowest block first
	// it is serialised BlockShort structure
	Height int
}

// Response of GetConsensusData request
type ComGetConsensusData struct {
	ConfigFile []byte
	Module     []byte // this can be long string
}

type ComGetData struct {
	AddrFrom netlib.NodeAddr
	Type     string
	ID       []byte
}

// Wallet Balance response
type ComWalletBalance struct {
	Total    float64
	Approved float64
	Pending  float64
}

// Request for a wallet balance
type ComGetWalletBalance struct {
	Address string
}

// New Transaction command. Is used by lite wallets
type ComNewTransaction struct {
	Address string
	TX      []byte
}

// New Transaction Data command. It includes prepared TX and signatures for imputs
type ComNewTransactionData struct {
	Address   string
	TX        []byte
	Signature []byte
}

// To Request new transaction by wallet.
// Wallet sends address where to send and amount to send
// and own pubkey. Server returns transaction but wihout signatures
type ComRequestTransaction struct {
	PubKey []byte
	To     string
	Amount float64
}

// To Request new SQL transaction by wallet.
// Wallet sends SQL command and own pubkey. Server returns transaction but wihout signatures
type ComRequestSQLTransaction struct {
	PubKey []byte
	SQL    string
}

// Response on prepare transaction request. Returns transaction without signs
// and data to sign
type ComRequestTransactionData struct {
	Finished   bool
	TX         []byte
	DataToSign []byte
}

// For request to get list of unspent transactions by wallet
type ComGetUnspentTransactions struct {
	Address   string
	LastBlock []byte
}

// Unspent Transaction record
type ComUnspentTransaction struct {
	TXID   []byte
	Vout   int
	Amount float64
	IsBase bool
	From   string
}

// Lit of unspent transactions returned on request
type ComUnspentTransactions struct {
	Transactions []ComUnspentTransaction
	LastBlock    []byte
}

// Request for history of transactions
type ComGetHistoryTransactions struct {
	Address string
}

// Record of transaction in list of history transactions
type ComHistoryTransaction struct {
	IOType bool // In (false) or Out (true)
	TXID   []byte
	Amount float64
	From   string
	To     string
}

// Request for inventory. It can be used to get blocks and transactions from other node
type ComInv struct {
	AddrFrom netlib.NodeAddr
	Type     string
	Items    [][]byte
}

// Transaction to send to other node
type ComTx struct {
	AddFrom     netlib.NodeAddr
	Transaction []byte // Transaction serialised
}

// Version mesage to other nodes
type ComVersion struct {
	Version    int
	BestHeight int
	AddrFrom   netlib.NodeAddr
}

// To send nodes manage command.
type ComManageNode struct {
	Node netlib.NodeAddr
}

// To get node state
type ComGetNodeState struct {
	Host                  string
	BlocksNumber          int
	ExpectingBlocksHeight int
	TransactionsCached    int
	UnspentOutputs        int
}

// To get node last updates
type ComGetUpdates struct {
	LastCheckTime      int64
	CurrentBlockHeight int
	TopBlocks          [][]byte
	AddrFrom           netlib.NodeAddr
}

// Response with updates on a node
type ResponseGetUpdates struct {
	CurrentBlockHeight      int
	CountTransactionsInPool int
	Blocks                  [][]byte
	TransactionsInPool      [][]byte
	Nodes                   []netlib.NodeAddrShort
}

// To get transaction from other node
type ComGetTransaction struct {
	TransactionID []byte
	AddrFrom      netlib.NodeAddr
}

// Response for transaction request
type ResponseGetTransaction struct {
	Transaction []byte // Transaction serialised
}

// Request to check if block exists. Executed before to send new block to node
type ComCheckBlock struct {
	BlockHash []byte
	AddrFrom  netlib.NodeAddr
}

// Response for check block request
type ResponseCheckBlock struct {
	Exists bool // True if a node doesn't want to get a body of this TX
}

// To get transaction from other node
type ComGetBlock struct {
	BlockHash []byte
	AddrFrom  netlib.NodeAddr
}

// Response for transaction request
type ResponseGetBlock struct {
	Block []byte // Transaction serialised
}

// Check if node address looks fine
func (c *NodeClient) SetAuthStr(auth string) {
	c.NodeAuthStr = auth
}

// Check if node address looks fine
func (c *NodeClient) CheckNodeAddress(address netlib.NodeAddr) error {
	if address.Port < 1024 {
		return errors.New("Node Address Port has wrong value")
	}
	if address.Port > 65536 {
		return errors.New("Node Address Port has wrong value")
	}
	if address.Host == "" {
		return errors.New("Node Address Host has wrong value")
	}
	return nil
}

// Set currrent node address , to include itin requests to other nodes
func (c *NodeClient) SetNodeAddress(address netlib.NodeAddr) {
	c.NodeAddress = address
}

// Send void commant to other node
// It is used by a node to send to itself only when we want to stop a node
// And unblock port listetining
func (c *NodeClient) SendVoid(address netlib.NodeAddr) error {
	request := netlib.CommandToBytes("viod")

	return c.SendData(address, request)
}

// Send list of nodes addresses to other node
func (c *NodeClient) SendAddrList(address netlib.NodeAddr, addresses []netlib.NodeAddr) error {
	data := ComAddresses{}
	data.Addresses = addresses
	data.AddrFrom = c.NodeAddress

	request, err := c.BuildCommandData(CommandAddresses, &data)

	if err != nil {
		return err
	}

	return c.SendData(address, request)
}

// Request for a block full info form other node
func (c *NodeClient) SendGetBlock(addr netlib.NodeAddr, blockHash []byte) (*ResponseGetBlock, error) {
	data := ComGetBlock{blockHash, c.NodeAddress}

	request, err := c.BuildCommandData(CommandGetBlock, &data)

	if err != nil {
		return nil, err
	}
	datapayload := ResponseGetBlock{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Send block to other node
func (c *NodeClient) SendBlock(addr netlib.NodeAddr, BlockSerialised []byte) error {
	data := ComBlock{c.NodeAddress, BlockSerialised}
	request, err := c.BuildCommandData(CommandBlock, &data)

	if err != nil {
		return err
	}

	return c.SendData(addr, request)
}

// Send inventory. Blocks hashes or transactions IDs
func (c *NodeClient) SendInv(address netlib.NodeAddr, kind string, items [][]byte) error {
	data := ComInv{c.NodeAddress, kind, items}

	request, err := c.BuildCommandData("inv", &data)

	if err != nil {
		return err
	}

	return c.SendData(address, request)
}

// Sedn request to get list of blocks on other node.
func (c *NodeClient) SendGetBlocks(address netlib.NodeAddr, startfrom []byte) error {
	data := ComGetBlocks{c.NodeAddress, startfrom}

	request, err := c.BuildCommandData("getblocks", &data)

	if err != nil {
		return err
	}

	return c.SendData(address, request)
}

// Request for blocks but result must be upper from some starting block
func (c *NodeClient) SendGetBlocksUpper(address netlib.NodeAddr, startfrom []byte) error {
	data := ComGetBlocks{c.NodeAddress, startfrom}

	request, err := c.BuildCommandData("getblocksup", &data)

	if err != nil {
		return err
	}

	return c.SendData(address, request)
}

// Request for list of first blocks in blockchain.
// This is used by new nodes
// TODO we can use SendGetBlocksUpper and empty hash. This will e same
func (c *NodeClient) SendGetFirstBlocks(address netlib.NodeAddr) (*ComGetFirstBlocksData, error) {
	request, err := c.BuildCommandData(CommandGetFirstBlocks, nil)

	if err != nil {
		return nil, err
	}
	datapayload := ComGetFirstBlocksData{}

	err = c.SendDataWaitResponse(address, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Request for consensus information from a node
func (c *NodeClient) SendGetConsensusData(address netlib.NodeAddr) (*ComGetConsensusData, error) {
	request, err := c.BuildCommandData(CommandGetConsensusData, nil)

	if err != nil {
		return nil, err
	}
	datapayload := ComGetConsensusData{}

	err = c.SendDataWaitResponse(address, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Request for a transaction or a block to get full info by ID or Hash
func (c *NodeClient) SendGetData(address netlib.NodeAddr, kind string, id []byte) error {

	data := ComGetData{c.NodeAddress, kind, id}

	request, err := c.BuildCommandData("getdata", &data)

	if err != nil {
		return err
	}

	return c.SendData(address, request)
}

// Get tranaction with sycn request. Wait response
func (c *NodeClient) SendGetTransaction(addr netlib.NodeAddr, txID []byte) (*ResponseGetTransaction, error) {
	data := ComGetTransaction{}
	data.TransactionID = txID
	data.AddrFrom = c.NodeAddress

	request, err := c.BuildCommandData(CommandGetTransaction, &data)

	if err != nil {
		return nil, err
	}

	datapayload := ResponseGetTransaction{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Check if tranaction exists on other node. To know if to send TX to a node in sync mode
func (c *NodeClient) SendCheckBlock(addr netlib.NodeAddr, hash []byte) (*ResponseCheckBlock, error) {
	data := ComCheckBlock{}
	data.BlockHash = hash
	data.AddrFrom = c.NodeAddress

	request, err := c.BuildCommandData(CommandCheckBlock, &data)

	if err != nil {
		return nil, err
	}

	datapayload := ResponseCheckBlock{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Send Transaction to other node
func (c *NodeClient) SendTx(addr netlib.NodeAddr, tnxserialised []byte) error {
	data := ComTx{c.NodeAddress, tnxserialised}
	request, err := c.BuildCommandData("tx", &data)

	if err != nil {
		return err
	}

	return c.SendData(addr, request)
}

// Send own version and blockchain state to other node
func (c *NodeClient) SendVersion(addr netlib.NodeAddr, bestHeight int) error {
	data := ComVersion{netlib.NodeVersion, bestHeight, c.NodeAddress}

	request, err := c.BuildCommandData("version", &data)

	if err != nil {
		return err
	}

	return c.SendData(addr, request)
}

// Request for history of transaction from a wallet
func (c *NodeClient) SendGetHistory(addr netlib.NodeAddr, address string) ([]ComHistoryTransaction, error) {
	data := ComGetHistoryTransactions{address}

	request, err := c.BuildCommandData("gethistory", &data)

	if err != nil {
		return nil, err
	}

	datapayload := []ComHistoryTransaction{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return datapayload, nil
}

// Send new transaction from a wallet to a node
func (c *NodeClient) SendNewTransactionData(addr netlib.NodeAddr, from string, txBytes []byte, signature []byte) ([]byte, error) {
	data := ComNewTransactionData{}
	data.Address = from
	data.TX = txBytes
	data.Signature = signature

	request, err := c.BuildCommandData("txdata", &data)

	NewTXID := []byte{}

	if err != nil {
		return nil, err
	}

	err = c.SendDataWaitResponse(addr, request, &NewTXID)

	if err != nil {
		return nil, err
	}
	// no data are returned. only success or not
	return NewTXID, nil
}

// Request to prepare new transaction by wallet.
// It returns a transaction without signature.
// Wallet has to sign it and then use SendNewTransaction to send completed transaction
func (c *NodeClient) SendRequestNewCurrencyTransaction(addr netlib.NodeAddr,
	PubKey []byte, to string, amount float64) ([]byte, []byte, error) {

	data := ComRequestTransaction{}
	data.PubKey = PubKey
	data.To = to
	data.Amount = amount

	request, err := c.BuildCommandData("txcurrequest", &data)

	if err != nil {
		return nil, nil, err
	}

	datapayload := ComRequestTransactionData{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, nil, err
	}

	return datapayload.TX, datapayload.DataToSign, nil
}

// Request to prepare new transaction by wallet.
// It returns a transaction without signature.
// Wallet has to sign it and then use SendNewTransaction to send completed transaction
func (c *NodeClient) SendRequestNewSQLTransaction(addr netlib.NodeAddr,
	PubKey []byte, sqlcommand string) (bool, []byte, []byte, error) {

	data := ComRequestSQLTransaction{}
	data.PubKey = PubKey
	data.SQL = sqlcommand

	request, err := c.BuildCommandData("txsqlrequest", &data)

	if err != nil {
		return false, nil, nil, err
	}

	datapayload := ComRequestTransactionData{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return false, nil, nil, err
	}

	return datapayload.Finished, datapayload.TX, datapayload.DataToSign, nil
}

// Request for list of unspent transactions outputs
// It can be used by wallet to see a state of balance
func (c *NodeClient) SendGetUnspent(addr netlib.NodeAddr, address string, chaintip []byte) (ComUnspentTransactions, error) {
	data := ComGetUnspentTransactions{address, chaintip}

	request, err := c.BuildCommandData("getunspent", &data)

	datapayload := ComUnspentTransactions{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return ComUnspentTransactions{}, err
	}

	return datapayload, nil
}

// Request for list of unspent transactions outputs
// It can be used by wallet to see a state of balance
func (c *NodeClient) SendGetBalance(addr netlib.NodeAddr, address string) (ComWalletBalance, error) {
	data := ComGetWalletBalance{address}

	request, err := c.BuildCommandData(CommandGetBalance, &data)

	datapayload := ComWalletBalance{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return ComWalletBalance{}, err
	}

	return datapayload, nil
}

// Request for list of nodes in contacts
func (c *NodeClient) SendGetNodes() ([]netlib.NodeAddr, error) {
	request, err := c.BuildCommandData("getnodes", nil)

	datapayload := []netlib.NodeAddr{}

	err = c.SendDataWaitResponse(c.NodeAddress, request, &datapayload)

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Get Nodes Response Error: %s", err.Error()))
	}

	return datapayload, nil
}

// Request to add new node to contacts
func (c *NodeClient) SendAddNode(node netlib.NodeAddr) error {
	data := ComManageNode{node}
	request, err := c.BuildCommandDataWithAuth("addnode", &data)

	err = c.SendDataWaitResponse(c.NodeAddress, request, nil)

	if err != nil {
		return errors.New(fmt.Sprintf("Add Node Response Error: %s", err.Error()))
	}

	return nil
}

// Request to remove a node from contacts
func (c *NodeClient) SendRemoveNode(node netlib.NodeAddr) error {
	data := ComManageNode{node}
	request, err := c.BuildCommandDataWithAuth("removenode", &data)

	err = c.SendDataWaitResponse(c.NodeAddress, request, nil)

	if err != nil {
		return errors.New(fmt.Sprintf("Remove Node Response Error: %s", err.Error()))
	}

	return nil
}

// Get node blockchain height
func (c *NodeClient) SendGetState() (ComGetNodeState, error) {
	request, err := c.BuildCommandDataWithAuth(CommandGetState, nil)

	data := ComGetNodeState{}

	err = c.SendDataWaitResponse(c.NodeAddress, request, &data)

	if err != nil {
		return data, errors.New(fmt.Sprintf("gettig state error: %s", err.Error()))
	}

	return data, nil
}

// Get last updates
func (c *NodeClient) SendGetUpdates(addr netlib.NodeAddr, lastCheckTime int64, blockHeight int, topBlocks [][]byte) (*ResponseGetUpdates, error) {
	data := ComGetUpdates{}
	data.LastCheckTime = lastCheckTime
	data.AddrFrom = c.NodeAddress
	data.CurrentBlockHeight = blockHeight
	data.TopBlocks = topBlocks

	request, err := c.BuildCommandData(CommandGetUpdates, &data)

	if err != nil {
		return nil, err
	}

	datapayload := ResponseGetUpdates{}

	err = c.SendDataWaitResponse(addr, request, &datapayload)

	if err != nil {
		return nil, err
	}

	return &datapayload, nil
}

// Builds a command data. It prepares a slice of bytes from given data
func (c *NodeClient) BuildCommandDataWithAuth(command string, data interface{}) ([]byte, error) {
	authbytes := netlib.CommandToBytes(c.NodeAuthStr)
	return c.doBuildCommandData(command, data, authbytes)
}

// Builds a command data. It prepares a slice of bytes from given data
func (c *NodeClient) BuildCommandData(command string, data interface{}) ([]byte, error) {
	return c.doBuildCommandData(command, data, []byte{})
}

// Builds a command data. It prepares a slice of bytes from given data
func (c *NodeClient) doBuildCommandData(command string, data interface{}, extra []byte) ([]byte, error) {
	var payload []byte
	var err error

	if data != nil {
		payload, err = netlib.GobEncode(data)

		if err != nil {
			return nil, err
		}
	} else {
		payload = []byte{}
	}
	//c.Logger.Trace.Printf("Build command %s", command)
	payloadlength := uint32(len(payload))
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, payloadlength) // convert int to []byte

	request := append(netlib.CommandToBytes(command), bs...)

	// add length of extra data
	payloadlength = uint32(len(extra))
	bs = make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, payloadlength) // convert int to []byte

	request = append(request, bs...)

	request = append(request, payload...)

	if len(extra) > 0 {
		request = append(request, extra...)
	}

	return request, nil
}

// Sends prepared command to a node. This doesn't wait any response
func (c *NodeClient) SendData(addr netlib.NodeAddr, data []byte) error {
	err := c.CheckNodeAddress(addr)

	if err != nil {
		return err
	}

	//c.Logger.Trace.Printf("Sending %d bytes to %s", len(data), addr.NodeAddrToString())
	conn, err := net.DialTimeout(netlib.Protocol, addr.NodeAddrToString(), 1*time.Second)

	if err != nil {
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Error: ", err.Error())

		// we can not connect.
		// we could remove this node from known
		// but this is not always good. we need somethign more smart here
		// TODO this needs analysis . if removing of a node is good idea
		//c.NodeNet.RemoveNodeFromKnown(addr)
		return netlib.NewCanNotConnectError(fmt.Sprintf("%s is not available", addr.NodeAddrToString()))
	}
	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))

	if err != nil {
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Error: ", err.Error())

		return netlib.NewCanNotSendError(err.Error())
	}
	return nil
}

// Send data to a node and wait for response
func (c *NodeClient) SendDataWaitResponse(addr netlib.NodeAddr, data []byte, datapayload interface{}) error {

	err := c.CheckNodeAddress(addr)

	if err != nil {
		c.Logger.Trace.Println("Wrong address " + addr.NodeAddrToString() + ": " + err.Error())
		return err
	}

	c.Logger.TraceExt.Println("Sending data to " + addr.NodeAddrToString() + " and waiting response")

	// connect
	dialer := net.Dialer{Timeout: time.Second * 2}
	conn, err := dialer.Dial(netlib.Protocol, addr.NodeAddrToString())

	if err != nil {
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Error: ", err.Error())

		// we can not connect.
		// we could remove this node from known
		// but this is not always good. we need somethign more smart here
		// TODO this needs analysis . if removing of a node is good idea
		//c.NodeNet.RemoveNodeFromKnown(addr)
		return netlib.NewCanNotConnectError(fmt.Sprintf("%s is not available", addr.NodeAddrToString()))
	}
	defer conn.Close()

	//c.Logger.Trace.Printf("Sending %d bytes ", len(data))
	// send command bytes
	_, err = io.Copy(conn, bytes.NewReader(data))

	if err != nil {
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Error: ", err.Error())
		return err
	}
	// read response
	// read everything
	//c.Logger.Trace.Println("Start readin response")

	response, err := ioutil.ReadAll(conn)

	if err != nil {
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Response Read Error: ", err.Error())
		return netlib.NewCanNotSendError(err.Error())
	}

	if len(response) == 0 {
		err := netlib.NewNoResponseError("Received 0 bytes as a response. Expected at least 1 byte")
		c.Logger.Error.Println(err.Error())
		c.Logger.Trace.Println("Response Read Error: ", err.Error())
		return err
	}

	c.Logger.TraceExt.Printf("Received %d bytes as a response\n", len(response))

	// convert response for provided structure
	var buff bytes.Buffer
	buff.Write(response[1:])
	dec := gob.NewDecoder(&buff)

	if response[0] != 1 {
		// fail

		var payload string

		err := dec.Decode(&payload)

		if err != nil {
			return netlib.NewCanNotParseResponseError(err.Error())
		}

		return errors.New(payload)
	}

	if datapayload != nil {
		err = dec.Decode(datapayload)

		if err != nil {
			return netlib.NewCanNotParseResponseError(err.Error())
		}
	}

	return nil
}
