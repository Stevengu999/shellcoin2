package connection

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/mesh/messages"
	"github.com/skycoin/skycoin/src/mesh/nodemanager"
)

func TestConnection(t *testing.T) {
	n := 4
	nm := nodemanager.NewNodeManager()

	nodes := nm.CreateNodeList(n)
	nm.Tick()
	nm.ConnectAll()

	node0 := nodes[0]
	route, backRoute, err := nm.BuildRoute(nodes)
	assert.Nil(t, err)
	conn0, err := NewConnectionWithRoutes(nm, node0, route, backRoute)
	conn0.Tick()
	assert.Nil(t, err)
	payload := []byte{'t', 'e', 's', 't'}
	msg := messages.RequestMessage{
		0,
		backRoute,
		payload,
	}
	msgS := messages.Serialize(messages.MsgRequestMessage, msg)
	sequence, err := conn0.Send(msgS)
	assert.Nil(t, err)
	assert.Equal(t, uint32(0), sequence)
	time.Sleep(time.Duration(n) * time.Second)
}
