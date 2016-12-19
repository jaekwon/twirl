package node

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/jaekwon/twirl/types"
	. "github.com/tendermint/go-common"
	cfg "github.com/tendermint/go-config"
	"github.com/tendermint/go-crypto"
	"github.com/tendermint/go-p2p"
	"github.com/tendermint/go-wire"

	_ "net/http/pprof"
)

type Node struct {
	config      cfg.Config
	sw          *p2p.Switch
	dataReactor *DataReactor
	privKey     crypto.PrivKeyEd25519
}

func NewNode(config cfg.Config) *Node {

	// Generate node PrivKey
	privKey := crypto.GenPrivKeyEd25519()

	// Make DataReactor
	dataReactor := NewDataReactor()
	input := config.GetString("input")
	output := config.GetString("output")

	if input != "" {
		// Load the file
		data, err := ReadFile(input)
		if err != nil {
			Exit(Fmt("Error reading file %v: %v", input, err))
		}

		// Compute parts
		parts := types.NewPartSetFromData(data, 1024*16)
		hash := parts.Hash()
		fmt.Printf("Hash: %X\n", hash)
		fmt.Println("PartSet.Header: ", parts.Header())

		dataReactor.SetParts(parts)
	}

	if output != "" {
		dataReactor.SetOutputPath(output)
	}

	// Make p2p network switch
	sw := p2p.NewSwitch(config.GetConfig("p2p"))
	sw.AddReactor("DATA", dataReactor)

	return &Node{
		config:      config,
		sw:          sw,
		dataReactor: dataReactor,
		privKey:     privKey,
	}
}

// Call Start() after adding the listeners.
func (n *Node) Start() error {
	n.sw.SetNodeInfo(makeNodeInfo(n.config, n.sw, n.privKey))
	n.sw.SetNodePrivKey(n.privKey)
	_, err := n.sw.Start()
	return err
}

func (n *Node) Stop() {
	fmt.Println("Stopping Node")
	// TODO: gracefully disconnect from peers.
	n.sw.Stop()
}

// Add a Listener to accept inbound peer connections.
// Add listeners before starting the Node.
// The first listener is the primary listener (in NodeInfo)
func (n *Node) AddListener(l p2p.Listener) {
	fmt.Println(Fmt("Added %v", l))
	n.sw.AddListener(l)
}

func (n *Node) Switch() *p2p.Switch {
	return n.sw
}

func (n *Node) DataReactor() *DataReactor {
	return n.dataReactor
}

func makeNodeInfo(config cfg.Config, sw *p2p.Switch, privKey crypto.PrivKeyEd25519) *p2p.NodeInfo {

	nodeInfo := &p2p.NodeInfo{
		PubKey:  privKey.PubKey().(crypto.PubKeyEd25519),
		Network: config.GetString("network"),
		Version: config.GetString("version"),
		Other: []string{
			Fmt("wire_version=%v", wire.Version),
			Fmt("p2p_version=%v", p2p.Version),
		},
	}

	if !sw.IsListening() {
		return nodeInfo
	}

	p2pListener := sw.Listeners()[0]
	p2pHost := p2pListener.ExternalAddress().IP.String()
	p2pPort := p2pListener.ExternalAddress().Port

	nodeInfo.ListenAddr = Fmt("%v:%v", p2pHost, p2pPort)
	return nodeInfo
}

//------------------------------------------------------------------------------

func RunNode(config cfg.Config) {
	// Create & start node
	n := NewNode(config)

	protocol, address := ProtocolAndAddress(config.GetString("node_laddr"))
	l := p2p.NewDefaultListener(protocol, address, config.GetBool("skip_upnp"))
	n.AddListener(l)
	err := n.Start()
	if err != nil {
		Exit(Fmt("Failed to start node: %v", err))
	}

	fmt.Println("Started node", "nodeInfo", n.sw.NodeInfo())

	// If seedNode is provided by config, dial out.
	if config.GetString("seeds") != "" {
		seeds := strings.Split(config.GetString("seeds"), ",")
		// Limit number of seeds?
		if limit := config.GetInt("seeds-limit"); limit != 0 && len(seeds) > limit {
			// Shuffle seeds
			for i := range seeds {
				j := rand.Intn(i + 1)
				seeds[i], seeds[j] = seeds[j], seeds[i]
			}
			// Get first `limit` seeds after shuffle
			seeds = seeds[:limit]
		}
		n.sw.DialSeeds(seeds)
	}

	// Sleep forever and then...
	TrapSignal(func() {
		n.Stop()
	})
}

func (n *Node) NodeInfo() *p2p.NodeInfo {
	return n.sw.NodeInfo()
}

func (n *Node) DialSeeds(seeds []string) {
	n.sw.DialSeeds(seeds)
}

// Defaults to tcp
func ProtocolAndAddress(listenAddr string) (string, string) {
	protocol, address := "tcp", listenAddr
	parts := strings.SplitN(address, "://", 2)
	if len(parts) == 2 {
		protocol, address = parts[0], parts[1]
	}
	return protocol, address
}
