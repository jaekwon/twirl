package node

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/jaekwon/twirl/types"
	. "github.com/tendermint/go-common"
	"github.com/tendermint/go-p2p"
	"github.com/tendermint/go-wire"
)

const (
	DataChannel                 = byte(0x20)
	broadcastStateSleepDuration = 2 * time.Second
	peerGossipSleepDuration     = 100 * time.Millisecond
	peerStateKey                = "peerState"
	maxDataMessageSize          = 1024 * 100 // 100KB
)

//-----------------------------------------------------------------------------

type DataReactor struct {
	p2p.BaseReactor // BaseService + p2p.Switch

	mtx    sync.Mutex
	parts  *types.PartSet
	header types.PartSetHeader // PartsHeader, if we have it.
}

func NewDataReactor() *DataReactor {
	reactor := &DataReactor{}
	reactor.BaseReactor = *p2p.NewBaseReactor(nil, "DataReactor", reactor)
	return reactor
}

func (reactor *DataReactor) OnStart() error {
	reactor.BaseReactor.OnStart()
	go reactor.broadcastStateRoutine()
	return nil
}

func (reactor *DataReactor) OnStop() {
	reactor.BaseReactor.OnStop()
}

func (reactor *DataReactor) SetPartsHeader(header types.PartSetHeader) {
	reactor.mtx.Lock()
	defer reactor.mtx.Unlock()

	if !reactor.header.IsZero() {
		return
	}

	reactor.parts = types.NewPartSetFromHeader(header)
	reactor.header = header
}

func (reactor *DataReactor) GetPartsHeader() types.PartSetHeader {
	reactor.mtx.Lock()
	defer reactor.mtx.Unlock()
	return reactor.header
}

func (reactor *DataReactor) SetParts(parts *types.PartSet) {
	reactor.mtx.Lock()
	defer reactor.mtx.Unlock()

	if reactor.parts != nil {
		return
	}

	reactor.parts = parts
	reactor.header = parts.Header()
}

func (reactor *DataReactor) GetParts() *types.PartSet {
	reactor.mtx.Lock()
	defer reactor.mtx.Unlock()
	return reactor.parts
}

// Implements Reactor
func (reactor *DataReactor) GetChannels() []*p2p.ChannelDescriptor {
	// TODO optimize
	return []*p2p.ChannelDescriptor{
		&p2p.ChannelDescriptor{
			ID:                DataChannel,
			Priority:          5,
			SendQueueCapacity: 100,
		},
	}
}

// Implements Reactor
func (reactor *DataReactor) AddPeer(peer *p2p.Peer) {
	if !reactor.IsRunning() {
		return
	}

	// Create peerState for peer
	peerState := NewPeerState(peer)
	peer.Data.Set(peerStateKey, peerState)

	// Begin routines for this peer.
	go reactor.gossipDataRoutine(peer, peerState)

}

// Implements Reactor
func (reactor *DataReactor) RemovePeer(peer *p2p.Peer, reason interface{}) {
	if !reactor.IsRunning() {
		return
	}
	// TODO
	//peer.Data.Get(PeerStateKey).(*PeerState).Disconnect()
}

// Implements Reactor
func (reactor *DataReactor) Receive(chID byte, src *p2p.Peer, msgBytes []byte) {
	if !reactor.IsRunning() {
		fmt.Println("Receive", "src", src, "chId", chID, "bytes", msgBytes)
		return
	}

	_, msg, err := DecodeMessage(msgBytes)
	if err != nil {
		fmt.Println("Error decoding message", "src", src, "chId", chID, "msg", msg, "error", err, "bytes", msgBytes)
		// TODO punish peer?
		return
	}
	fmt.Println("Receive", "src", src, "chId", chID, "msg", msg)

	// Get peer states
	ps := src.Data.Get(peerStateKey).(*PeerState)

	// Handle message
	ps.DidSendMessage(msg)
	switch msg := msg.(type) {
	case *PartsHeaderMessage:
		if reactor.GetPartsHeader().IsZero() {
			reactor.SetPartsHeader(msg.Header)
		}
	case *PartMessage:
		if parts := reactor.GetParts(); parts != nil {
			added, _ := parts.AddPart(msg.Part, true)
			if added {
				msg := HasPartMessage{
					Index: msg.Part.Index,
				}
				reactor.Switch.Broadcast(DataChannel, struct{ DataMessage }{msg})
			}
			fmt.Println(Fmt("Added %v of %v", parts.Count(), parts.Total()))
		}
	default:
		fmt.Println(Fmt("Unknown message type %v", reflect.TypeOf(msg)))
	}

	if err != nil {
		fmt.Println("Error in Receive()", "error", err)
	}
}

func (reactor *DataReactor) broadcastStateRoutine() {
	for {
		// Manage disconnects from self.
		if !reactor.IsRunning() {
			return
		}

		// Maybe broadcast PartsHeaderMessage
		header := reactor.GetPartsHeader()
		if !header.IsZero() {
			msg := PartsHeaderMessage{
				Header: header,
			}
			reactor.Switch.Broadcast(DataChannel, struct{ DataMessage }{msg})
		}

		// Maybe broadcast HasParts
		parts := reactor.GetParts()
		if parts != nil {
			msg := HasPartsMessage{
				BitArray: parts.BitArray(),
			}
			reactor.Switch.Broadcast(DataChannel, struct{ DataMessage }{msg})
		}

		time.Sleep(broadcastStateSleepDuration)
	}
}

func (reactor *DataReactor) gossipDataRoutine(peer *p2p.Peer, ps *PeerState) {

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !reactor.IsRunning() {
			fmt.Println(Fmt("Stopping gossipDataRoutine for %v.", peer))
			return
		}

		// Pick a part to send
		psHasParts := ps.GetHasParts()
		if psHasParts != nil {
			pick, ok := reactor.parts.BitArray().Sub(psHasParts).PickRandom()
			if ok {
				// Send the part
				part := reactor.parts.GetPart(pick)
				msg := &PartMessage{
					Part: part,
				}
				if peer.TrySend(DataChannel, struct{ DataMessage }{msg}) {
					ps.SetHasPart(pick)
				}
				continue OUTER_LOOP
			}
		}

		// Nothing to do. Sleep.
		time.Sleep(peerGossipSleepDuration)
		continue OUTER_LOOP
	}
}

//--------------------------------------------------------------------------------

type PeerState struct {
	Peer *p2p.Peer

	mtx      sync.Mutex
	hasParts *BitArray
}

func NewPeerState(peer *p2p.Peer) *PeerState {
	return &PeerState{
		Peer:     peer,
		hasParts: nil,
	}
}

func (ps *PeerState) DidSendMessage(msg DataMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	switch msg := msg.(type) {
	case *PartsHeaderMessage:
		// ignore
	case *HasPartsMessage:
		ps.hasParts = msg.BitArray.Copy()
	case *HasPartMessage:
		ps.hasParts.SetIndex(msg.Index, true)
	case *PartMessage:
		ps.hasParts.SetIndex(msg.Part.Index, true)
	default:
		fmt.Println(Fmt("Unknown message type %v", reflect.TypeOf(msg)))
	}
}

func (ps *PeerState) SetHasPart(index int) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.hasParts.SetIndex(index, true)
}

func (ps *PeerState) GetHasParts() *BitArray {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.hasParts.Copy()
}

func (ps *PeerState) String() string {
	return ps.StringIndented("")
}

func (ps *PeerState) StringIndented(indent string) string {
	return fmt.Sprintf(`PeerState{
%s  Key %v
%s  HasParts %v
%s}`,
		indent, ps.Peer.Key,
		indent, ps.hasParts.String(),
		indent)
}

//-----------------------------------------------------------------------------
// Messages

const (
	msgTypePartsHeader = byte(0x01)
	msgTypeHasParts    = byte(0x02)
	msgTypeHasPart     = byte(0x03)
	msgTypePart        = byte(0x04)
)

type DataMessage interface{}

var _ = wire.RegisterInterface(
	struct{ DataMessage }{},
	wire.ConcreteType{&PartsHeaderMessage{}, msgTypePartsHeader},
	wire.ConcreteType{&HasPartsMessage{}, msgTypeHasParts},
	wire.ConcreteType{&HasPartMessage{}, msgTypeHasPart},
	wire.ConcreteType{&PartMessage{}, msgTypePart},
)

// TODO: check for unnecessary extra bytes at the end.
func DecodeMessage(bz []byte) (msgType byte, msg DataMessage, err error) {
	msgType = bz[0]
	n := new(int)
	r := bytes.NewReader(bz)
	msg = wire.ReadBinary(struct{ DataMessage }{}, r, maxDataMessageSize, n, &err).(struct{ DataMessage }).DataMessage
	return
}

//-------------------------------------

type PartsHeaderMessage struct {
	Header types.PartSetHeader
}

func (m *PartsHeaderMessage) String() string {
	return fmt.Sprintf("[PartsHeader %v]", m.Header)
}

//-------------------------------------

type HasPartsMessage struct {
	*BitArray
}

func (m *HasPartsMessage) String() string {
	return fmt.Sprintf("[HasParts %v]", m.BitArray)
}

//-------------------------------------

type HasPartMessage struct {
	Index int
}

func (m *HasPartMessage) String() string {
	return fmt.Sprintf("[HasPart %v]", m.Index)
}

//-------------------------------------

type PartMessage struct {
	*types.Part
}

func (m *PartMessage) String() string {
	return fmt.Sprintf("[Part %v]", m.Part)
}
